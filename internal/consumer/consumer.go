package consumer

import (
	"context"
	"log"

	"github.com/Nurasick/transactional-notification-service/internal/notification"
	amqp "github.com/rabbitmq/amqp091-go"
)

const emailQueue = "notifications.email"

type Consumer struct {
	channel    *amqp.Channel
	dispatcher *notification.Dispatcher
}

func New(channel *amqp.Channel, dispatcher *notification.Dispatcher) *Consumer {
	return &Consumer{channel: channel, dispatcher: dispatcher}
}

// Run starts consuming messages and blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		emailQueue,
		"",    // consumer tag — auto-generated
		false, // auto-ack — we ack manually after processing
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		return err
	}

	log.Printf("[CONSUMER] listening on queue: %s", emailQueue)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			c.handle(ctx, msg)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg amqp.Delivery) {
	err := c.dispatcher.Handle(ctx, msg.MessageId, msg.Body)
	if err != nil {
		log.Printf("[CONSUMER] failed message_id=%s err=%v — requeueing", msg.MessageId, err)
		msg.Nack(false, true) // requeue=true
		return
	}
	msg.Ack(false)
}
