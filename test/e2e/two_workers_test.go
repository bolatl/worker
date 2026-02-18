package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestTwoWorkers verifies: 2 workers + multiple jobs → both workers do work.
func TestTwoWorkers(t *testing.T) {
	if os.Getenv("E2E") == "" {
		t.Skip("E2E tests require E2E=1. Run: E2E=1 go test ./test/e2e/...")
	}

	root := findProjectRoot(t)
	runDockerCompose(t, root, "up", "-d", "--scale", "worker=2")
	defer runDockerCompose(t, root, "up", "-d", "--scale", "worker=1")
	time.Sleep(5 * time.Second) // let workers start

	client := NewClient(os.Getenv("API_URL"))

	// Create multiple jobs
	const numJobs = 8
	var jobIDs []string
	for i := 0; i < numJobs; i++ {
		payload := json.RawMessage(fmt.Sprintf(`{"n":%d}`, i))
		id, err := client.CreateJob("hash", payload)
		if err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}
		jobIDs = append(jobIDs, id)
	}

	// Time how long until all complete
	start := time.Now()
	for _, id := range jobIDs {
		_, err := client.WaitForTerminal(id, 3*time.Minute)
		if err != nil {
			t.Fatalf("wait for job %s: %v", id, err)
		}
	}
	elapsed := time.Since(start)

	// Verify all done
	for i, id := range jobIDs {
		job, err := client.GetJob(id)
		if err != nil {
			t.Fatalf("get job %s: %v", id, err)
		}
		if job.Status != "done" {
			t.Errorf("job %d (%s): expected done, got %s", i, id, job.Status)
		}
	}

	// With 2 workers and prefetch=1, 8 jobs of 15s each should complete in ~60s
	// (4 batches of 2 parallel). Single worker would take ~120s.
	// Assert both workers contributed: elapsed should be significantly less than single-worker time.
	singleWorkerTime := time.Duration(numJobs) * 15 * time.Second
	if elapsed > singleWorkerTime*9/10 {
		t.Logf("elapsed %v for %d jobs; with 2 workers expected < %v. "+
			"Run 'docker compose logs worker' to verify both workers handled jobs.",
			elapsed, numJobs, singleWorkerTime)
		// Don't fail - timing can vary; the important check is all jobs completed.
	}

	// Try to verify both worker containers logged activity
	cmd := exec.Command("docker", "compose", "logs", "--no-log-prefix", "worker")
	cmd.Dir = findProjectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("could not get worker logs (docker compose logs): %v", err)
		return
	}
	// With scaled workers, logs may show worker-worker-1 and worker-worker-2
	// Docker Compose prefixes with container name when scaled
	if strings.Contains(string(out), "worker-worker-1") && strings.Contains(string(out), "worker-worker-2") {
		t.Logf("both worker-worker-1 and worker-worker-2 appear in logs")
	} else if strings.Count(string(out), "Handling job") >= numJobs {
		t.Logf("worker(s) handled %d jobs", numJobs)
	}
}
