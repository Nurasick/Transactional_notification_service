package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nurasick/transactional-notification-service/internal/broker"
	"github.com/Nurasick/transactional-notification-service/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublisher struct {
	err    error
	called bool
	got    broker.Event
}

func (f *fakePublisher) Publish(_ context.Context, event broker.Event) error {
	f.called = true
	f.got = event
	return f.err
}

type fakeNotifLister struct {
	err     error
	records []store.NotificationRecord
	got     store.ListFilter
}

func (f *fakeNotifLister) List(_ context.Context, fil store.ListFilter) ([]store.NotificationRecord, error) {
	f.got = fil
	return f.records, f.err
}

func TestPublishHandler_WrongMethod(t *testing.T) {
	pub := &fakePublisher{}
	req := httptest.NewRequest(http.MethodGet, "/publish", nil)
	rec := httptest.NewRecorder()

	publishHandler(pub).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.False(t, pub.called)
}

func TestPublishHandler_BadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing user id",
			body: `{"event_type":"order.placed"}`,
		},
		{
			name: "not json",
			body: "not json",
		},
		{
			name: "missing event type",
			body: `{"user_id":"1"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pub := &fakePublisher{}
			req := httptest.NewRequest(http.MethodPost, "/publish", strings.NewReader(test.body))
			rec := httptest.NewRecorder()

			publishHandler(pub).ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.False(t, pub.called)
		})
	}
}

func TestPublishHandler_PublishReturnsError(t *testing.T) {
	pub := &fakePublisher{
		err: errors.New("boom"),
	}
	body := `{"user_id":"1","event_type":"order.placed"}`
	req := httptest.NewRequest(http.MethodPost, "/publish", strings.NewReader(body))
	rec := httptest.NewRecorder()

	publishHandler(pub).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.True(t, pub.called)
}

func TestPublishHandler_HappyPath(t *testing.T) {
	pub := &fakePublisher{}
	body := `{"user_id":"1","event_type":"order.placed","payload":{"OrderID":"ORD-42"}}`
	req := httptest.NewRequest(http.MethodPost, "/publish", strings.NewReader(body))
	rec := httptest.NewRecorder()

	publishHandler(pub).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	_, err := uuid.Parse(resp["message_id"])
	require.NoError(t, err)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, pub.got.MessageID, resp["message_id"])
	assert.Equal(t, "1", pub.got.UserID)
	assert.Equal(t, "order.placed", pub.got.EventType)
	assert.Equal(t, "ORD-42", pub.got.Payload["OrderID"])
}

func TestListHandler_WrongMethod(t *testing.T) {
	lister := &fakeNotifLister{}
	req := httptest.NewRequest(http.MethodPost, "/notifications", nil)
	rec := httptest.NewRecorder()

	listHandler(lister).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, store.ListFilter{}, lister.got)
}

func TestListHandler_QueryParams(t *testing.T) {
	lister := &fakeNotifLister{}
	req := httptest.NewRequest(http.MethodGet, "/notifications?user_id=u1&status=sent&event_type=order.placed&limit=5&offset=10", nil)
	rec := httptest.NewRecorder()

	listHandler(lister).ServeHTTP(rec, req)

	want := store.ListFilter{
		UserID:    "u1",
		Status:    "sent",
		EventType: "order.placed",
		Limit:     5,
		Offset:    10,
	}
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, want, lister.got)
}

// Documents current behavior: bad numbers are passed to the store unchanged
// instead of being rejected. N11 should turn these into 400s.
func TestListHandler_BadNumbers(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "limit is not a number",
			target:     "/notifications?limit=abc&offset=10",
			wantLimit:  0,
			wantOffset: 10,
		},
		{
			name:       "offset negative",
			target:     "/notifications?limit=5&offset=-1",
			wantLimit:  5,
			wantOffset: -1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lister := &fakeNotifLister{}
			req := httptest.NewRequest(http.MethodGet, test.target, nil)
			rec := httptest.NewRecorder()

			listHandler(lister).ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, test.wantLimit, lister.got.Limit)
			assert.Equal(t, test.wantOffset, lister.got.Offset)
		})
	}
}

func TestListHandler_StoreError(t *testing.T) {
	lister := &fakeNotifLister{err: errors.New("boom")}
	req := httptest.NewRequest(http.MethodGet, "/notifications?user_id=u1&status=sent&event_type=order.placed&limit=5&offset=10", nil)
	rec := httptest.NewRecorder()

	listHandler(lister).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestListHandler_HappyPath(t *testing.T) {
	lister := &fakeNotifLister{
		records: []store.NotificationRecord{
			{MessageID: "m1", UserID: "u1", Status: "sent"},
			{MessageID: "m2", UserID: "u1", Status: "failed"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/notifications?user_id=u1", nil)
	rec := httptest.NewRecorder()

	listHandler(lister).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp["data"], 2)
	assert.Equal(t, float64(2), resp["count"]) // JSON numbers decode to float64
	assert.Equal(t, float64(0), resp["offset"])
}

// Documents current behavior: an empty result is encoded as "data": null
// instead of []. N11 should change this to an empty array.
func TestListHandler_EmptyResult(t *testing.T) {
	lister := &fakeNotifLister{}
	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	rec := httptest.NewRecorder()

	listHandler(lister).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp["data"])
	assert.Equal(t, float64(0), resp["count"])
}
