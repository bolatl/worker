package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "http://localhost:8080"

// Client talks to the jobs API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a client with default timeouts.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CreateJobReq is the request body for POST /jobs.
type CreateJobReq struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// CreateJobResp is the response from POST /jobs.
type CreateJobResp struct {
	JobID string `json:"job_id"`
	Error string `json:"error,omitempty"`
}

// JobStatus is the response from GET /jobs/:id.
type JobStatus struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Attempts  int             `json:"attempts"`
	MaxAttempts int           `json:"max_attempts"`
	LastError *string         `json:"last_error,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

// CreateJob creates a job and returns the job ID.
func (c *Client) CreateJob(typ string, payload json.RawMessage) (string, error) {
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	body, _ := json.Marshal(CreateJobReq{Type: typ, Payload: payload})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/jobs", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var createResp CreateJobResp
	if err := json.Unmarshal(data, &createResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if createResp.Error != "" {
		return "", fmt.Errorf("api error: %s", createResp.Error)
	}
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}
	return createResp.JobID, nil
}

// GetJob fetches job status.
func (c *Client) GetJob(id string) (*JobStatus, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/jobs/"+id, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var job JobStatus
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("job not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}
	return &job, nil
}

// WaitForTerminal polls until job reaches done or failed, or timeout.
func (c *Client) WaitForTerminal(id string, timeout time.Duration) (*JobStatus, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := c.GetJob(id)
		if err != nil {
			return nil, err
		}
		if job.Status == "done" || job.Status == "failed" {
			return job, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for job %s to reach terminal state", id)
}
