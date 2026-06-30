// Package httpapi exposes the local browser UI and core HTTP API routes.
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

const serviceName = "magichandy"

// VersionInfo identifies the build served by the HTTP API.
type VersionInfo struct {
	Version string
	Commit  string
}

// Server owns the Phase 1 HTTP routes and embedded static asset serving.
type Server struct {
	static  fs.FS
	logger  *slog.Logger
	started time.Time
	version VersionInfo
	handler http.Handler
}

// New wires the HTTP API to the embedded static assets and structured logger.
func New(static fs.FS, logger *slog.Logger, version VersionInfo) (*Server, error) {
	if static == nil {
		return nil, errors.New("static filesystem is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	server := &Server{
		static:  static,
		logger:  logger,
		started: time.Now().UTC(),
		version: version,
	}

	mux := http.NewServeMux()
	server.routes(mux)
	server.handler = logRequests(logger, mux)

	return server, nil
}

// Handler returns the HTTP handler for use by net/http servers and tests.
func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /", s.handleStatic)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   serviceName,
		"version":   s.version.Version,
		"commit":    s.version.Commit,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":        serviceName,
		"version":        s.version.Version,
		"commit":         s.version.Commit,
		"uptime_seconds": int64(time.Since(s.started).Seconds()),
		"ui":             "embedded",
		"features": map[string]string{
			"chat":      "not_implemented",
			"motion":    "not_implemented",
			"transport": "not_implemented",
			"voice":     "not_implemented",
		},
	})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := cleanAssetName(r.URL.Path)
	data, err := fs.ReadFile(s.static, name)
	if err != nil {
		if strings.Contains(path.Base(name), ".") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		data, err = fs.ReadFile(s.static, name)
		if err != nil {
			http.Error(w, "embedded UI is unavailable", http.StatusInternalServerError)
			return
		}
	}

	setStaticHeaders(w, name)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func cleanAssetName(urlPath string) string {
	name := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if name == "." || name == "" {
		return "index.html"
	}
	return name
}

func setStaticHeaders(w http.ResponseWriter, name string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	switch path.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		panic(fmt.Errorf("encode JSON response: %w", err))
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
