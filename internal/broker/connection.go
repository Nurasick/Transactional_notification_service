package broker

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchangeName = "notifications"
	emailQueue   = "notifications.email"
	dlqQueue     = "notifications.deadletter"
)

type Connection struct {
	conn    *amqp.Connection
	Channel *amqp.Channel
}

func Connect(url string) (*Connection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}

	if err := setup(ch); err != nil {
		ch.Close()
		conn.Close()
		return nil, err
	}

	return &Connection{conn: conn, Channel: ch}, nil
}

func (c *Connection) Close() {
	c.Channel.Close()
	c.conn.Close()
}

func setup(ch *amqp.Channel) error {
	// Declare the topic exchange
	if err := ch.ExchangeDeclare(
		exchangeName, // name
		"topic",      // kind
		true,         // durable
		false,        // auto-delete
		false,        // internal
		false,        // no-wait
		nil,          // args
	); err != nil {
		return err
	}

	// Dead-letter queue — simple, no special args
	if _, err := ch.QueueDeclare(dlqQueue, true, false, false, false, nil); err != nil {
		return err
	}

	// Main queue — routes failed messages to DLQ
	if _, err := ch.QueueDeclare(
		emailQueue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-dead-letter-exchange":     "",
			"x-dead-letter-routing-key":  dlqQueue,
		},
	); err != nil {
		return err
	}

	// Bind main queue to exchange — "#" matches every routing key
	return ch.QueueBind(emailQueue, "#", exchangeName, false, nil)
}
