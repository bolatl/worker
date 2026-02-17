package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Rabbit struct {
	conn      *amqp.Connection
	ch        *amqp.Channel
	queueName string
}

func Connect(url, queueName string) (*Rabbit, error) {
	if url == "" {
		return nil, fmt.Errorf("RABBIT_URL is empty")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	// Durable queue so messages survive broker restarts
	_, err = ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return &Rabbit{conn: conn, ch: ch, queueName: queueName}, nil
}

func (r *Rabbit) Close() {
	if r.ch != nil {
		_ = r.ch.Close()
	}
	if r.conn != nil {
		_ = r.conn.Close()
	}
}

func (r *Rabbit) PublishJob(ctx context.Context, jobID string) error {
	body, err := json.Marshal(JobMessage{JobID: jobID})
	if err != nil {
		return err
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.ch.PublishWithContext(
		pubCtx,
		"",          // default exchange
		r.queueName, // routing key = queue name
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // survive broker restart
		},
	)
}

// Consume returns deliveries with manual acks.
func (r *Rabbit) Consume(prefetch int) (<-chan amqp.Delivery, error) {
	if prefetch <= 0 {
		prefetch = 1
	}
	if err := r.ch.Qos(prefetch, 0, false); err != nil {
		return nil, err
	}
	return r.ch.Consume(
		r.queueName,
		"",    // consumer tag
		false, // autoAck = false (manual ack required)
		false,
		false,
		false,
		nil,
	)
}
