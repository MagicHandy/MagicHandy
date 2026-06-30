package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHealthzReturnsOK(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body field = %v, want ok", body["status"])
	}
	if body["service"] != serviceName {
		t.Fatalf("service body field = %v, want %s", body["service"], serviceName)
	}
}

func TestStatusAdvertisesPhaseOnePlaceholders(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body struct {
		UI       string            `json:"ui"`
		Features map[string]string `json:"features"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if body.UI != "embedded" {
		t.Fatalf("ui = %q, want embedded", body.UI)
	}
	if body.Features["motion"] != "not_implemented" {
		t.Fatalf("motion feature = %q, want not_implemented", body.Features["motion"])
	}
}

func TestStaticShellIsServed(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q, want text/html; charset=utf-8", got)
	}
}

func TestMissingAssetReturnsNotFound(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	server, err := New(fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>MagicHandy</title>")},
		"app.css":    {Data: []byte("body { margin: 0; }")},
		"app.js":     {Data: []byte("console.log('ready');")},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), VersionInfo{
		Version: "test",
		Commit:  "test",
	})
	if err != nil {
		t.Fatalf("New server: %v", err)
	}
	return server
}
