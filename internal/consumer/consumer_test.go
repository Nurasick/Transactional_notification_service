package consumer

import (
	"context"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

type fakeAcknowledger struct {
	err        error
	ackCalled  bool
	nackCalled bool
	requeue    bool
}

func (f *fakeAcknowledger) Ack(tag uint64, multiple bool) error {
	f.ackCalled = true
	return f.err
}
func (f *fakeAcknowledger) Nack(tag uint64, multiple, requeue bool) error {
	f.nackCalled = true
	f.requeue = requeue
	return f.err
}
func (f *fakeAcknowledger) Reject(tag uint64, requeue bool) error {
	f.requeue = requeue
	return f.err
}

type fakeDispatcher struct {
	err          error
	gotMessageID string
	gotBody      []byte
}

func (f *fakeDispatcher) Handle(_ context.Context, messageID string, body []byte) error {
	f.gotMessageID = messageID
	f.gotBody = body
	return f.err
}

func TestHandle_Success(t *testing.T) {
	ack := &fakeAcknowledger{}
	msg := amqp.Delivery{
		Acknowledger: ack,
		MessageId:    "m1",
		Body:         []byte(`{"event_type":"order.placed"}`),
	}
	disp := &fakeDispatcher{}

	c := New(nil, disp)

	c.handle(context.Background(), msg)

	assert.True(t, ack.ackCalled)
	assert.False(t, ack.nackCalled)
	assert.Equal(t, "m1", disp.gotMessageID)
	assert.Equal(t, []byte(`{"event_type":"order.placed"}`), disp.gotBody)
}

func TestHandle_DispatchError(t *testing.T) {
	ack := &fakeAcknowledger{}
	msg := amqp.Delivery{
		Acknowledger: ack,
		MessageId:    "m1",
		Body:         []byte(`{"event_type":"order.placed"}`),
	}
	disp := &fakeDispatcher{
		err: errors.New("boom"),
	}
	c := New(nil, disp)
	c.handle(context.Background(), msg)
	assert.False(t, ack.ackCalled)
	assert.True(t, ack.nackCalled)
	assert.True(t, ack.requeue)
}
