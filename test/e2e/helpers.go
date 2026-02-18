package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func findProjectRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

// runDockerCompose runs docker compose in root; fails test on error.
func runDockerCompose(t *testing.T, root string, args ...string) {
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("docker compose %v: %v\n%s", args, err, out)
		t.Fatalf("docker compose failed")
	}
}
