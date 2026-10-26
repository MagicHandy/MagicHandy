package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/voice"
)

func TestDiagnosticVoiceRequestsExcludePersonalContent(t *testing.T) {
	requests := []voice.RequestSnapshot{{ID: "asr-1", Role: voice.RoleASR, State: "failed", CompletionMillis: 1250,
		Transcript: []voice.TranscriptCandidate{{Text: "private transcript"}},
		Error:      &voice.WorkerError{Code: "private provider response", Message: "private prompt echoed by server", Retryable: true}},
		{ID: "tts-2", Role: voice.RoleTTS, State: "failed", AudioBytes: 24000, FirstAudioMillis: 300, Error: &voice.WorkerError{Code: voice.ErrorCodeTimeout}}}
	data, err := json.Marshal(diagnosticVoiceRequests(requests))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private") || strings.Contains(string(data), "transcript") {
		t.Fatalf("personal content in metadata: %s", data)
	}
	for _, wanted := range []string{`"completion_ms":1250`, `"first_audio_ms":300`, `"audio_bytes":24000`, `"error_code":"timeout"`, `"error_code":"provider_error"`} {
		if !strings.Contains(string(data), wanted) {
			t.Fatalf("missing %s in %s", wanted, data)
		}
	}
}
