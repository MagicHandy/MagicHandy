package httpapi

import (
	"context"
	"net/http"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

type emergencyStopRequest struct {
	Reason string `json:"reason"`
}

type statusCompatRuntime struct {
	mu            sync.Mutex
	emergencyStop bool
}

func (s *Server) handleHealthCompat(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *Server) handleEmergencyStop(w http.ResponseWriter, r *http.Request) {
	reason := "api_stop"
	var body emergencyStopRequest
	if r.ContentLength != 0 {
		if err := decodeOptionalJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if body.Reason != "" {
			reason = body.Reason
		}
	}

	s.setEmergencyStop(true)
	if s.modes != nil {
		s.modes.NotifyUserStop()
	}
	s.stopAndClearMotionEngine(r.Context(), reason)
	if client := s.intifaceClient(); client != nil {
		_ = client.StopAllDevices(r.Context())
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"emergency_stop": true,
	})
}

func (s *Server) handleEmergencyStopClear(w http.ResponseWriter, _ *http.Request) {
	s.setEmergencyStop(false)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"emergency_stop": false,
	})
}

func (s *Server) emergencyStopActive() bool {
	s.statusCompat.mu.Lock()
	defer s.statusCompat.mu.Unlock()
	return s.statusCompat.emergencyStop
}

func (s *Server) setEmergencyStop(active bool) {
	s.statusCompat.mu.Lock()
	s.statusCompat.emergencyStop = active
	s.statusCompat.mu.Unlock()
}

func (s *Server) startIntifaceReconnectLoop(ctx context.Context) {
	client := s.intifaceClient()
	if client == nil {
		return
	}
	client.StartReconnectLoop(ctx, func() bool {
		settings, _ := s.store.Snapshot()
		return settings.Device.HSPDispatchOwner == config.DispatchOwnerIntiface
	})
}
