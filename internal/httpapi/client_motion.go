package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) clientMotionState(r *http.Request) any {
	return s.clientMotionObservation(r, false)
}

func (s *Server) clientMotionObservation(r *http.Request, shared bool) any {
	value := s.motionObservation(shared)
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
	status, payload := stopReply(outcome, err)
	writeBoundedJSON(w, status, payload)
}

func stopStatus(outcome emergencyStopResult, err error) string {
	available, confirmed := stopConfirmation(outcome, err)
	switch {
	case outcome.pending:
		return "pending"
	case confirmed:
		return "confirmed"
	case !available && err == nil && outcome.transportResult.Status == "no_devices":
		return "no_device"
	default:
		return "unconfirmed"
	}
}

func stopReply(outcome emergencyStopResult, err error) (int, map[string]any) {
	available, confirmed := stopConfirmation(outcome, err)
	reply := map[string]any{"available": available, "stopped": !outcome.pending, "transport_stop_confirmed": confirmed}
	reply["stop_status"] = stopStatus(outcome, err)
	switch reply["stop_status"] {
	case "pending":
		reply["stop_pending"], reply["error"] = true, errStopConfirmationPending.Error()
		return http.StatusServiceUnavailable, reply
	case "unconfirmed":
		reply["error"] = "Stop is unconfirmed. Check the device locally."
		if available {
			return http.StatusBadGateway, reply
		}
	}
	return http.StatusOK, reply
}

func stopConfirmation(outcome emergencyStopResult, err error) (bool, bool) {
	result := outcome.transportResult
	if outcome.engineAvailable && outcome.state.LastResult != nil {
		result = *outcome.state.LastResult
	}
	available := outcome.engineAvailable || outcome.transportAvailable
	confirmed := err == nil && result.ConfirmsStop()
	return available, confirmed
}
