package notification

import (
	"context"
	"log"
)

type LogSender struct{}

func NewLogSender() *LogSender { return &LogSender{} }

func (s *LogSender) Send(ctx context.Context, n Notification) error {
	log.Printf("[NOTIFICATION] user=%s event=%s message_id=%s body=%s",
		n.UserID, n.EventType, n.MessageID, n.Body)
	return nil
}
