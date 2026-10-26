package openaittsworker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagedReadinessWaitsForLoadingModel(t *testing.T) {
	var calls atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"model_loaded":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"model_loaded":true}`))
	}))
	defer endpoint.Close()
	// Only the process-liveness boundary is faked; probes use real HTTP.
	s := &session{options: Options{BaseURL: endpoint.URL, HealthPath: "/health", HealthReadyField: "model_loaded", HTTPClient: endpoint.Client()},
		server: &managedServer{command: &exec.Cmd{}, done: make(chan error)}}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := s.waitReady(ctx); err != nil {
		t.Fatalf("normal loading was rejected: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("readiness polls = %d", calls.Load())
	}
}

func TestManagedReadinessStillRejectsInvalidContract(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"wrong_field":true}`)) }))
	defer endpoint.Close()
	s := &session{options: Options{BaseURL: endpoint.URL, HealthPath: "/health", HealthReadyField: "model_loaded", HTTPClient: endpoint.Client()},
		server: &managedServer{command: &exec.Cmd{}, done: make(chan error)}}
	if err := s.waitReady(t.Context()); !errors.Is(err, errHealthNotReady) || errors.Is(err, errHealthLoading) {
		t.Fatalf("invalid readiness contract: %v", err)
	}
}
