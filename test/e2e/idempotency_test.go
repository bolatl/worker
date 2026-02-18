package e2e

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// jobMsg matches the RabbitMQ message format.
type jobMsg struct {
	JobID string `json:"job_id"`
}

// TestDuplicateDeliveryIdempotency verifies: if the same job message is delivered
// more than once, the system remains consistent (no double-processing, no corruption).
func TestDuplicateDeliveryIdempotency(t *testing.T) {
	if os.Getenv("E2E") == "" {
		t.Skip("E2E tests require E2E=1. Run: E2E=1 go test ./test/e2e/...")
	}

	client := NewClient(os.Getenv("API_URL"))
	rabbitURL := getEnvDefault("RABBIT_URL", "amqp://guest:guest@localhost:5672/")
	queueName := getEnvDefault("QUEUE_NAME", "jobs")

	// 1. Create job and wait for it to complete
	payload := json.RawMessage(`{"idem_test": true}`)
	jobID, err := client.CreateJob("hash", payload)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	job, err := client.WaitForTerminal(jobID, 45*time.Second)
	if err != nil {
		t.Fatalf("wait for terminal: %v", err)
	}
	if job.Status != "done" {
		t.Fatalf("expected job done, got %s", job.Status)
	}
	originalResult := string(job.Result)
	if originalResult == "" {
		t.Fatal("expected non-empty result")
	}

	// 2. Publish a duplicate message to RabbitMQ (simulates at-least-once redelivery)
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatalf("connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	defer ch.Close()

	body, _ := json.Marshal(jobMsg{JobID: jobID})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = ch.PublishWithContext(ctx, "", queueName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
	if err != nil {
		t.Fatalf("publish duplicate: %v", err)
	}

	// 3. Wait for worker to receive and handle the duplicate
	time.Sleep(5 * time.Second)

	// 4. Verify job still done, result unchanged (idempotent - no re-processing)
	job2, err := client.GetJob(jobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job2.Status != "done" {
		t.Errorf("duplicate delivery corrupted state: expected status done, got %s", job2.Status)
	}
	if string(job2.Result) != originalResult {
		t.Errorf("duplicate delivery corrupted result: was %q, now %q", originalResult, string(job2.Result))
	}
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
