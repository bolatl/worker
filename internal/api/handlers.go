// Package api provides HTTP handlers for job creation, retrieval, and health checks.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"worker/internal/jobs"

	"github.com/jackc/pgx/v5"
)

// Handlers holds dependencies for HTTP request handlers.
type Handlers struct {
	svc  *jobs.Service
	repo *jobs.Repository
}

// NewHandlers creates Handlers with the given job service and repository.
func NewHandlers(svc *jobs.Service, repo *jobs.Repository) *Handlers {
	return &Handlers{svc: svc, repo: repo}
}

// createJobReq is the JSON body for job creation.
type createJobReq struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// CreateJob handles POST /jobs: creates a job and publishes it to the queue.
func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var req createJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "type is required"})
		return
	}
	if len(req.Payload) == 0 {
		req.Payload = json.RawMessage(`{}`)
	}

	id, err := h.svc.CreateJob(ctx, req.Type, req.Payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

// GetJob handles GET /jobs/{id}: returns job details by ID, or 404 if not found.
func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	j, err := h.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			fmt.Fprintln(w, "not found")
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// return payload/result as raw json (bytes)
	resp := map[string]any{
		"id": j.ID, "type": j.Type, "status": j.Status,
		"attempts": j.Attempts, "max_attempts": j.MaxAttempts,
		"created_at": j.CreatedAt, "updated_at": j.UpdatedAt,
	}

	if len(j.Payload) > 0 {
		resp["payload"] = json.RawMessage(j.Payload)
	}
	if len(j.Result) > 0 {
		resp["result"] = json.RawMessage(j.Result)
	}
	if j.LastError != nil {
		resp["last_error"] = *j.LastError
	}

	writeJSON(w, http.StatusOK, resp)
}

// Healthz handles GET /healthz: returns 200 OK for liveness/readiness probes.
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// writeJSON sets Content-Type to application/json, writes status, and encodes v as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
