package broker

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Event struct {
	MessageID string         `json:"message_id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	UserID    string         `json:"user_id"`
}

func (c *Connection) Publish(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return c.Channel.PublishWithContext(ctx, exchangeName, event.EventType, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    event.MessageID,
		Body:         body,
	})
}
