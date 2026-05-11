package notification

import "context"

type Notification struct {
	MessageID string
	UserID    string
	EventType string
	Body      string
}

type Sender interface {
	Send(ctx context.Context, n Notification) error
}
