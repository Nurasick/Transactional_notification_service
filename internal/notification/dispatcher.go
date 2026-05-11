package notification

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/Nurasick/transactional-notification-service/internal/store"
)

// interfaces let us swap real Redis/MinIO/Postgres for fakes in tests.

type deduplicator interface {
	IsDuplicate(ctx context.Context, id string) (bool, error)
}

type limiter interface {
	Allow(ctx context.Context, userID string) (bool, error)
}

type templateRenderer interface {
	Render(ctx context.Context, eventType string, data map[string]any) (string, error)
}

type notificationLogger interface {
	Insert(ctx context.Context, r store.NotificationRecord) error
}

type Dispatcher struct {
	dedup     deduplicator
	limiter   limiter
	templates templateRenderer
	sender    Sender
	log       notificationLogger
}

func NewDispatcher(
	dedup deduplicator,
	limiter limiter,
	templates templateRenderer,
	sender Sender,
	log notificationLogger,
) *Dispatcher {
	return &Dispatcher{
		dedup:     dedup,
		limiter:   limiter,
		templates: templates,
		sender:    sender,
		log:       log,
	}
}

type incomingEvent struct {
	MessageID string         `json:"message_id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	UserID    string         `json:"user_id"`
}

func (d *Dispatcher) Handle(ctx context.Context, messageID string, body []byte) error {
	var event incomingEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}

	isDup, err := d.dedup.IsDuplicate(ctx, messageID)
	if err != nil {
		return err
	}
	if isDup {
		log.Printf("[DUPLICATE] message_id=%s", messageID)
		return d.writeRecord(ctx, messageID, event, "duplicate", "")
	}

	allowed, err := d.limiter.Allow(ctx, event.UserID)
	if err != nil {
		return err
	}
	if !allowed {
		log.Printf("[RATE_LIMITED] user_id=%s", event.UserID)
		return d.writeRecord(ctx, event.MessageID, event, "rate_limited", "")
	}

	rendered, err := d.templates.Render(ctx, event.EventType, event.Payload)
	if err != nil {
		d.writeRecord(ctx, messageID, event, "failed", err.Error())
		return err
	}

	if err := d.sender.Send(ctx, Notification{
		MessageID: messageID,
		UserID:    event.UserID,
		EventType: event.EventType,
		Body:      rendered,
	}); err != nil {
		d.writeRecord(ctx, messageID, event, "failed", err.Error())
		return err
	}

	return d.writeRecord(ctx, messageID, event, "sent", "")
}

func (d *Dispatcher) writeRecord(ctx context.Context, messageID string, event incomingEvent, status, errMsg string) error {
	return d.log.Insert(ctx, store.NotificationRecord{
		MessageID: messageID,
		UserID:    event.UserID,
		EventType: event.EventType,
		Status:    status,
		Error:     errMsg,
		SentAt:    time.Now(),
	})
}
