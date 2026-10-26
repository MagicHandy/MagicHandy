package httpapi

import "github.com/mapledaemon/MagicHandy/internal/voice"

// Retain timing and failure categories for support without silently opting the
// export into transcripts, spoken reply text, reference audio or provider bodies.
type diagnosticVoiceRequest struct {
	ID               string     `json:"id"`
	Role             voice.Role `json:"role"`
	State            string     `json:"state"`
	CreatedAt        string     `json:"created_at"`
	AudioChunks      int        `json:"audio_chunks,omitempty"`
	AudioBytes       int        `json:"audio_bytes,omitempty"`
	FirstAudioMillis int64      `json:"first_audio_ms,omitempty"`
	CompletionMillis int64      `json:"completion_ms,omitempty"`
	ErrorCode        string     `json:"error_code,omitempty"`
	Retryable        bool       `json:"retryable,omitempty"`
}

func diagnosticVoiceRequests(requests []voice.RequestSnapshot) []diagnosticVoiceRequest {
	result := make([]diagnosticVoiceRequest, 0, len(requests))
	for _, request := range requests {
		item := diagnosticVoiceRequest{ID: request.ID, Role: request.Role, State: request.State, CreatedAt: request.CreatedAt,
			AudioChunks: request.AudioChunks, AudioBytes: request.AudioBytes, FirstAudioMillis: request.FirstAudioMillis, CompletionMillis: request.CompletionMillis}
		if request.Error != nil {
			item.Retryable = request.Error.Retryable
			switch request.Error.Code {
			case voice.ErrorCodeProtocolMismatch, voice.ErrorCodeInvalidRequest, voice.ErrorCodeModelNotLoaded, voice.ErrorCodeMissingDependency, voice.ErrorCodeCanceled, voice.ErrorCodeTimeout, voice.ErrorCodeInternal:
				item.ErrorCode = request.Error.Code
			default:
				item.ErrorCode = "provider_error"
			}
		}
		result = append(result, item)
	}
	return result
}
