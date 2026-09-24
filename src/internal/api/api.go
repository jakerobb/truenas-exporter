package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jakerobb/truenas-exporter/internal/metrics"
)

// Server exposes /metrics and /health.
type Server struct {
	collector *metrics.Collector
	port      int
}

// New creates a new API Server.
func New(collector *metrics.Collector, port int) *Server {
	return &Server{collector: collector, port: port}
}

// Start registers routes and begins listening. It blocks until the server fails.
func (srv *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", srv.handleHealth)
	mux.HandleFunc("GET /metrics", srv.handleMetrics)

	server := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", srv.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}

// handleHealth reports only that the process is serving; TrueNAS
// reachability is reported by truenas_up instead, so a NAS outage doesn't
// get this pod restarted.
func (srv *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(`{"status":"ok"}`))
	if err != nil {
		slog.Error("failed to write health response", "err", err)
	}
}

func (srv *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	snap := srv.collector.Collect(r.Context())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if err := metrics.Write(w, snap); err != nil {
		slog.Error("failed to write metrics response", "err", err)
	}
}
