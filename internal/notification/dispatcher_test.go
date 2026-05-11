package notification_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Nurasick/transactional-notification-service/internal/notification"
	"github.com/Nurasick/transactional-notification-service/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- fakes ----

type fakeDedup struct{ dup bool }

func (f *fakeDedup) IsDuplicate(_ context.Context, _ string) (bool, error) { return f.dup, nil }

type fakeLimiter struct{ allow bool }

func (f *fakeLimiter) Allow(_ context.Context, _ string) (bool, error) { return f.allow, nil }

type fakeTemplates struct{ body string; err error }

func (f *fakeTemplates) Render(_ context.Context, _ string, _ map[string]any) (string, error) {
	return f.body, f.err
}

type fakeSender struct{ err error; calls atomic.Int64 }

func (f *fakeSender) Send(_ context.Context, _ notification.Notification) error {
	f.calls.Add(1)
	return f.err
}

type fakeLogger struct{ last store.NotificationRecord; err error }

func (f *fakeLogger) Insert(_ context.Context, r store.NotificationRecord) error {
	f.last = r
	return f.err
}

// ---- helpers ----

func makeBody(t *testing.T, msgID, userID, eventType string) []byte {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"message_id": msgID,
		"user_id":    userID,
		"event_type": eventType,
		"payload":    map[string]any{"UserName": "Nurali"},
	})
	return b
}

func newDispatcher(dedup *fakeDedup, lim *fakeLimiter, tmpl *fakeTemplates, sender *fakeSender, logger *fakeLogger) *notification.Dispatcher {
	return notification.NewDispatcher(dedup, lim, tmpl, sender, logger)
}

// ---- tests ----

func TestDispatcher_HappyPath_Sent(t *testing.T) {
	logger := &fakeLogger{}
	sender := &fakeSender{}
	d := newDispatcher(
		&fakeDedup{dup: false},
		&fakeLimiter{allow: true},
		&fakeTemplates{body: "Hello Nurali"},
		sender,
		logger,
	)

	err := d.Handle(context.Background(), "msg-1", makeBody(t, "msg-1", "u1", "order.placed"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), sender.calls.Load())
	assert.Equal(t, "sent", logger.last.Status)
}

func TestDispatcher_DuplicateMessage_Skipped(t *testing.T) {
	logger := &fakeLogger{}
	sender := &fakeSender{}
	d := newDispatcher(
		&fakeDedup{dup: true},
		&fakeLimiter{allow: true},
		&fakeTemplates{body: "x"},
		sender,
		logger,
	)

	err := d.Handle(context.Background(), "msg-dup", makeBody(t, "msg-dup", "u1", "order.placed"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), sender.calls.Load(), "sender must not be called for duplicates")
	assert.Equal(t, "duplicate", logger.last.Status)
}

func TestDispatcher_RateLimited_NotSent(t *testing.T) {
	logger := &fakeLogger{}
	sender := &fakeSender{}
	d := newDispatcher(
		&fakeDedup{dup: false},
		&fakeLimiter{allow: false},
		&fakeTemplates{body: "x"},
		sender,
		logger,
	)

	err := d.Handle(context.Background(), "msg-rl", makeBody(t, "msg-rl", "u1", "order.placed"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), sender.calls.Load())
	assert.Equal(t, "rate_limited", logger.last.Status)
}

func TestDispatcher_TemplateError_RecordsFailedStatus(t *testing.T) {
	logger := &fakeLogger{}
	d := newDispatcher(
		&fakeDedup{dup: false},
		&fakeLimiter{allow: true},
		&fakeTemplates{err: errors.New("template not found")},
		&fakeSender{},
		logger,
	)

	err := d.Handle(context.Background(), "msg-tmpl", makeBody(t, "msg-tmpl", "u1", "unknown.event"))
	assert.Error(t, err)
	assert.Equal(t, "failed", logger.last.Status)
	assert.Contains(t, logger.last.Error, "template not found")
}

func TestDispatcher_SenderError_RecordsFailedStatus(t *testing.T) {
	logger := &fakeLogger{}
	d := newDispatcher(
		&fakeDedup{dup: false},
		&fakeLimiter{allow: true},
		&fakeTemplates{body: "ok"},
		&fakeSender{err: errors.New("smtp down")},
		logger,
	)

	err := d.Handle(context.Background(), "msg-send", makeBody(t, "msg-send", "u1", "order.placed"))
	assert.Error(t, err)
	assert.Equal(t, "failed", logger.last.Status)
	assert.Equal(t, "smtp down", logger.last.Error)
}

func TestDispatcher_InvalidJSON_ReturnsError(t *testing.T) {
	d := newDispatcher(&fakeDedup{}, &fakeLimiter{}, &fakeTemplates{}, &fakeSender{}, &fakeLogger{})
	err := d.Handle(context.Background(), "msg-bad", []byte("not json"))
	assert.Error(t, err)
}

// BenchmarkDispatcher_HappyPath measures dispatcher throughput with all fakes.
func BenchmarkDispatcher_HappyPath(b *testing.B) {
	d := newDispatcher(
		&fakeDedup{dup: false},
		&fakeLimiter{allow: true},
		&fakeTemplates{body: "Hello"},
		&fakeSender{},
		&fakeLogger{},
	)
	body, _ := json.Marshal(map[string]any{
		"message_id": "bench-msg",
		"user_id":    "u1",
		"event_type": "order.placed",
		"payload":    map[string]any{},
	})
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		d.Handle(ctx, "bench-msg", body)
	}
}
