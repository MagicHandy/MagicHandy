package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

var errLabsDisabled = errors.New("labs are disabled; enable them in Settings > General")

func (s *Server) handleSetLabsEnabled(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Enabled == nil {
		writeError(w, http.StatusBadRequest, errors.New("enabled is required"))
		return
	}
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		current.Labs.Enabled = *body.Enabled
		return current, nil
	})
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, errors.New("labs setting could not be saved"))
		return
	}
	if runtimeErr != nil {
		writeError(w, http.StatusBadGateway, errors.New("labs setting was saved, but the active lab audition could not be stopped cleanly"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": saved.Labs.Enabled})
}

// Every lab endpoint uses the persisted flag and registers cancelable work.
// Cancels are synchronous under lab.mu, the same lock used to commit trials,
// so a late provider cannot update a preview after disable has completed.
func (s *Server) beginLabRequest(parent context.Context) (context.Context, func(), error) {
	s.lab.mu.Lock()
	defer s.lab.mu.Unlock()
	settings, _ := s.store.Snapshot()
	if !settings.Labs.Enabled {
		return nil, nil, errLabsDisabled
	}
	ctx, cancel := context.WithCancel(parent)
	if s.lab.requests == nil {
		s.lab.requests = make(map[uint64]context.CancelFunc)
	}
	s.lab.nextRequest++
	id := s.lab.nextRequest
	s.lab.requests[id] = cancel
	return ctx, func() {
		cancel()
		s.lab.mu.Lock()
		delete(s.lab.requests, id)
		s.lab.mu.Unlock()
	}, nil
}

func (s *Server) withLabs(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, finish, err := s.beginLabRequest(r.Context())
		if err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
		defer finish()
		handler(w, r.WithContext(ctx))
	}
}

func (s *Server) beginLabMotion(parent context.Context) (context.Context, func(), error) {
	ctx, finish, err := s.beginLabRequest(parent)
	if err != nil {
		return nil, nil, err
	}
	s.lab.startMu.Lock()
	settings, _ := s.store.Snapshot()
	if ctx.Err() != nil || !settings.Labs.Enabled {
		s.lab.startMu.Unlock()
		finish()
		return nil, nil, errLabsDisabled
	}
	return ctx, func() { s.lab.startMu.Unlock(); finish() }, nil
}

func (s *Server) disableLabs(ctx context.Context) error {
	finishLab := s.cancelLabSession()
	defer finishLab()
	s.lab.mu.Lock()
	for _, cancel := range s.lab.requests {
		cancel()
	}
	s.lab.mu.Unlock()
	// Wait for a canceled audition start to drain before inspecting the engine.
	// New starts recheck the setting after taking this lock. Regular motion and
	// Emergency Stop never need this lab-only admission lock.
	s.lab.startMu.Lock()
	defer s.lab.startMu.Unlock()
	s.motion.lifecycleMu.Lock()
	defer s.motion.lifecycleMu.Unlock()
	engine := s.currentMotionEngine()
	if engine == nil || engine.Snapshot().Target.Source != motion.TargetSourceMotionLab {
		return nil
	}
	return s.stopAndClearMotionEngineLocked(ctx, "labs_disabled")
}
