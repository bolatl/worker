package e2e

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestHappyPath verifies: create job → becomes done.
func TestHappyPath(t *testing.T) {
	if os.Getenv("E2E") == "" {
		t.Skip("E2E tests require E2E=1. Run: E2E=1 go test ./test/e2e/...")
	}

	client := NewClient(os.Getenv("API_URL"))

	// Create a normal job (type hash, will succeed)
	payload := json.RawMessage(`{"test": true}`)
	jobID, err := client.CreateJob("hash", payload)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if jobID == "" {
		t.Fatal("expected non-empty job_id")
	}

	// Wait for done (WORK_DURATION=15s in compose, allow 2x for safety)
	job, err := client.WaitForTerminal(jobID, 45*time.Second)
	if err != nil {
		t.Fatalf("wait for terminal: %v", err)
	}

	if job.Status != "done" {
		t.Fatalf("expected status done, got %s (attempts=%d, last_error=%v)",
			job.Status, job.Attempts, job.LastError)
	}
	if len(job.Result) == 0 {
		t.Error("expected non-empty result")
	}

	// Result should contain sha256 hash
	var result map[string]any
	if err := json.Unmarshal(job.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if _, ok := result["sha256"]; !ok {
		t.Error("expected result to contain sha256")
	}
}
