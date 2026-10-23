package httpapi

import (
	"errors"
	"net/http"
)

func (s *Server) clientChatEmitter(w http.ResponseWriter, r *http.Request) sseEmitter {
	return func(event string, payload any) error {
		if s.capabilities(r).ConfigureHost {
			return writeSSE(w, event, payload)
		}
		view, err := s.clientChatEvent(r, event, payload)
		if err != nil {
			return err
		}
		return writeSSE(w, event, view)
	}
}

// Conversation text, semantic motion and reconciliation IDs are shared data.
// Provider configuration, raw failures and inference diagnostics are host data.
// New event shapes must choose an audience instead of inheriting raw payloads.
func (s *Server) clientChatEvent(r *http.Request, event string, payload any) (any, error) {
	switch event {
	case "status":
		return chatEventFields(payload, "state", "session_id", "user_seq", "current_mood", "persona_id", "persona_name", "stop_sequence")
	case "delta", "repair_delta":
		return chatEventFields(payload, "phase", "text")
	case "message":
		return chatEventFields(payload, "reply", "repaired", "semantic_fallback", "initial_malformed", "motion", "new_mood", "current_mood", "seq")
	case "speech":
		return chatEventFields(payload, "request_id")
	case "done":
		return chatEventFields(payload, "ok", "repaired", "semantic_fallback", "malformed")
	case "malformed":
		view, err := chatEventFields(payload, "repaired", "recoverable", "phase")
		if err != nil {
			return nil, err
		}
		view["error"] = administratorDetails
		return view, nil
	case "error":
		view := map[string]any{"message": clientChatFailure(payload)}
		if values, ok := payload.(map[string]string); ok && values["reply_retained"] == "true" {
			view["reply_retained"] = "true"
		}
		if s.capabilities(r).Control {
			if values, ok := payload.(map[string]any); ok {
				if repair, ok := values["repair"].(map[string]any); ok {
					id, validID := repair["failure_id"].(int64)
					available, _ := repair["available"].(bool)
					if validID && id > 0 && available {
						view["message"] = "The model response was rejected. You can ask it to correct the response."
						view["repair"] = map[string]any{"failure_id": id, "available": true}
					}
				}
			}
		}
		return view, nil
	case "motion":
		dispatch, ok := payload.(chatMotionDispatch)
		if !ok {
			return nil, errors.New("unreviewed shared motion event shape")
		}
		dispatch.Engine = s.clientMotionSnapshot(r, dispatch.Engine)
		dispatch.Error = clientFailure(dispatch.Error)
		return dispatch, nil
	default:
		return nil, errors.New("unreviewed shared chat event")
	}
}

func chatEventFields(payload any, fields ...string) (map[string]any, error) {
	values, ok := payload.(map[string]any)
	if !ok {
		return nil, errors.New("unreviewed shared chat payload shape")
	}
	view := make(map[string]any, len(fields))
	for _, name := range fields {
		if value, ok := values[name]; ok {
			view[name] = value
		}
	}
	return view, nil
}

func clientChatFailure(payload any) string {
	values, _ := payload.(map[string]string)
	message := values["message"]
	if fields, ok := payload.(map[string]any); ok {
		message, _ = fields["message"].(string)
	}
	if sharedChatInterruption(message) {
		return message
	}
	switch message {
	case "Chat canceled by Emergency Stop.", "Chat history is unavailable; the reply was not applied.", "Response repair canceled.":
		return message
	default:
		if message == errChatRepairStale.Error() {
			return message
		}
		return administratorDetails
	}
}
