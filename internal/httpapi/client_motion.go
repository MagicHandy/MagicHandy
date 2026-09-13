package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (s *Server) clientMotionState(r *http.Request) any {
	value := s.motionState().(map[string]any)
	if s.capabilities(r).ConfigureHost {
		return value
	}
	if message, ok := value["error"].(string); ok {
		value["error"] = clientFailure(message)
	}
	if state, ok := value["engine"].(motion.ActiveMotionState); ok {
		value["engine"] = s.clientMotionSnapshot(r, state)
	}
	return value
}

// Engine state is semantic shared content. Transport errors/results are host
// diagnostics, so they cannot ride along in a shared snapshot or command reply.
func (s *Server) clientMotionSnapshot(r *http.Request, state motion.ActiveMotionState) motion.ActiveMotionState {
	if !s.capabilities(r).ConfigureHost {
		state.LastError = clientFailure(state.LastError)
		state.LastResult = nil
	}
	return state
}

func (s *Server) clientSyncStatus(r *http.Request, state mediaSyncStatus) mediaSyncStatus {
	if !s.capabilities(r).ConfigureHost && state.State == "error" {
		state.Message = clientFailure(state.Message)
	}
	return state
}

// The public Stop lane intentionally skips authentication/database admission.
// Its acknowledgement must therefore be equally safe for every caller, even
// when the browser supplied a cookie. Never add the previous target/settings.
func (s *Server) writePublicStopResult(w http.ResponseWriter, outcome emergencyStopResult, err error) {
	result := outcome.transportResult
	if outcome.engineAvailable && outcome.state.LastResult != nil {
		result = *outcome.state.LastResult
	}
	available := outcome.engineAvailable || outcome.transportAvailable
	confirmed := err == nil && result.Kind == transport.CommandKindStop && result.OK
	payload := map[string]any{"available": available, "stopped": true, "transport_stop_confirmed": confirmed}
	status := http.StatusOK
	if err != nil {
		payload["error"] = "Stop is unconfirmed. Check the device locally."
		if available {
			status = http.StatusBadGateway
		}
	}
	writeJSON(w, status, payload)
}
