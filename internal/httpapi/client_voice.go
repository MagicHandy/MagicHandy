package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/voice"
)

func (s *Server) clientVoiceState(r *http.Request) map[string]any {
	if s.capabilities(r).ConfigureHost {
		return s.voiceState()
	}
	settings, _ := s.store.Snapshot()
	workers := s.voice.Status()
	for role, status := range workers {
		workers[role] = voice.WorkerStatus{
			Role: status.Role, State: status.State, Configured: status.Configured,
			ProtocolVersion: status.ProtocolVersion, ModelState: clientModelState(status.ModelState),
			QueueDepth: status.QueueDepth, WorkerQueue: status.WorkerQueue,
			ActiveRequestID: status.ActiveRequestID, StartedAt: status.StartedAt,
			LastError: clientFailure(status.LastError),
		}
	}
	// No module-path inspection is needed for an operational observer snapshot.
	return map[string]any{"enabled": settings.Voice.Enabled, "protocol_version": voice.ProtocolVersion, "workers": workers}
}

func clientModelState(state string) string {
	switch state {
	case "", "loading", "loaded", "unloaded", "failed", "ready", "unknown":
		return state
	default:
		return "unknown"
	}
}

func clientFailure(message string) string {
	if message == "" {
		return ""
	}
	return administratorDetails
}

func (s *Server) clientVoiceRequest(r *http.Request, snapshot voice.RequestSnapshot) voice.RequestSnapshot {
	if s.capabilities(r).ConfigureHost {
		return snapshot
	}
	view := voice.RequestSnapshot{
		ID: snapshot.ID, Role: snapshot.Role, Type: snapshot.Type, State: snapshot.State,
		CreatedAt: snapshot.CreatedAt, AudioChunks: snapshot.AudioChunks, AudioBytes: snapshot.AudioBytes,
		AudioFormat: snapshot.AudioFormat, FirstAudioMillis: snapshot.FirstAudioMillis,
		CompletionMillis: snapshot.CompletionMillis, AudioTruncated: snapshot.AudioTruncated,
		Transcript: snapshot.Transcript, Rejected: clientFailure(snapshot.Rejected),
	}
	if snapshot.Error != nil {
		view.Error = &voice.WorkerError{Code: "request_failed", Message: administratorDetails, Retryable: snapshot.Error.Retryable}
	}
	return view
}

func (s *Server) clientVoiceRequests(r *http.Request) []voice.RequestSnapshot {
	requests := s.voice.Requests()
	for i := range requests {
		requests[i] = s.clientVoiceRequest(r, requests[i])
	}
	return requests
}
