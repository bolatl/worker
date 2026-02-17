package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"worker/internal/queue"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	deliveries <-chan amqp.Delivery
	processor  *Processor
}

func NewConsumer(deliveries <-chan amqp.Delivery, processor *Processor) *Consumer {
	return &Consumer{deliveries: deliveries, processor: processor}
}

func (c *Consumer) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-c.deliveries:
			if !ok {
				return nil
			}
			c.handleOne(ctx, d)
		}
	}
}

func (c *Consumer) handleOne(ctx context.Context, d amqp.Delivery) {
	var msg queue.JobMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil || msg.JobID == "" {
		_ = d.Ack(false) // bad message -> ack to drop
		return
	}

	// Process with timeout per message (optional, but helps)
	msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	terminal, err := c.processor.Handle(msgCtx, msg.JobID)
	if err == nil && terminal {
		_ = d.Ack(false)
		return
	}

	// If terminal (poison job reached failed), we ACK to stop retries.
	if terminal {
		_ = d.Ack(false)
		return
	}

	// Non-terminal failure: NACK requeue for retry.
	// Simple small delay to avoid hot loop (minimal requirement).
	time.Sleep(500 * time.Millisecond)

	if err != nil {
		log.Printf("job %s error: %v (requeue)", msg.JobID, err)
	}
	_ = d.Nack(false, true) // requeue=true
}
