package e2e

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestPoisonJob verifies: submit job that fails → reaches failed eventually, no infinite loop.
func TestPoisonJob(t *testing.T) {
	if os.Getenv("E2E") == "" {
		t.Skip("E2E tests require E2E=1. Run: E2E=1 go test ./test/e2e/...")
	}

	client := NewClient(os.Getenv("API_URL"))

	// Create a poison job (type "fail" always fails)
	jobID, err := client.CreateJob("fail", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// With max_attempts=5 and WORK_DURATION=15s, expect ~75s max.
	// Add buffer for NACK delays and reaper.
	job, err := client.WaitForTerminal(jobID, 2*time.Minute)
	if err != nil {
		t.Fatalf("wait for terminal: %v", err)
	}

	if job.Status != "failed" {
		t.Fatalf("expected status failed, got %s (attempts=%d, max_attempts=%d)",
			job.Status, job.Attempts, job.MaxAttempts)
	}
	if job.Attempts < job.MaxAttempts {
		t.Errorf("expected attempts >= max_attempts, got attempts=%d max_attempts=%d",
			job.Attempts, job.MaxAttempts)
	}
	if job.LastError == nil || *job.LastError == "" {
		t.Error("expected last_error to be set")
	}
}
