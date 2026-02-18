package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestWorkerCrash verifies: kill worker mid-job → restart → job completes or fails cleanly (no silent loss).
func TestWorkerCrash(t *testing.T) {
	if os.Getenv("E2E") == "" {
		t.Skip("E2E tests require E2E=1. Run: E2E=1 go test ./test/e2e/...")
	}

	root := findProjectRoot(t)
	client := NewClient(os.Getenv("API_URL"))

	// Ensure exactly 1 worker for predictable test
	scaleDown(t, root)
	defer scaleUp(t, root)

	// Create a job (15s work duration - worker will be killed during work)
	jobID, err := client.CreateJob("hash", json.RawMessage(`{"crash_test": true}`))
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// Give worker time to pick up the job and start processing
	time.Sleep(2 * time.Second)

	// Kill the worker mid-processing
	killWorkers(t, root)

	// Restart worker
	startWorkers(t, root, 1)

	// Wait for job to reach terminal state (reaper runs every 10s, timeout 60s,
	// so stuck job may take up to 70s to be requeued, then 15s to process)
	job, err := client.WaitForTerminal(jobID, 2*time.Minute)
	if err != nil {
		t.Fatalf("wait for terminal: %v", err)
	}

	if job.Status != "done" && job.Status != "failed" {
		t.Fatalf("expected terminal state (done or failed), got %s", job.Status)
	}
	// Job must not be lost (still processing after 2 min would indicate a bug)
}

func scaleDown(t *testing.T, root string) {
	runDockerCompose(t, root, "up", "-d", "--scale", "worker=0")
	time.Sleep(2 * time.Second) // let worker shut down
	runDockerCompose(t, root, "up", "-d", "--scale", "worker=1")
	time.Sleep(3 * time.Second) // let worker start
}

func scaleUp(t *testing.T, root string) {
	runDockerCompose(t, root, "up", "-d", "--scale", "worker=1")
}

func killWorkers(t *testing.T, root string) {
	cmd := exec.Command("docker", "compose", "kill", "worker")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Logf("docker compose kill worker: %v (may already be down)", err)
	}
	time.Sleep(2 * time.Second)
}

func startWorkers(t *testing.T, root string, replicas int) {
	runDockerCompose(t, root, "up", "-d", "--scale", "worker=1")
	time.Sleep(5 * time.Second) // wait for worker to be ready
}
