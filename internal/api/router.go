package api

import "net/http"

// Router builds an http.Handler with routes for jobs (POST/GET) and healthz.
func Router(h *Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /jobs", h.CreateJob)
	mux.HandleFunc("GET /jobs/{id}", h.GetJob)
	mux.HandleFunc("GET /healthz", h.Healthz)

	return mux
}
