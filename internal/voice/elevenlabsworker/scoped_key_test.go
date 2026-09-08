package elevenlabsworker

import (
	"bytes"
	"context"
	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTTSScopeKeyDoesNotRequireAccountAccess(t *testing.T) {
	var output bytes.Buffer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/user" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = io.WriteString(w, "synthetic audio")
	}))
	defer srv.Close()
	// #nosec G101 -- deliberately fake credential used only by the local httptest server.
	s := &session{options: Options{BaseURL: srv.URL, HTTPClient: srv.Client(), APIKey: "non-secret-fixture", VoiceID: "fixture", ModelID: "fixture", OutputFormat: "mp3_44100_128"}, writer: &output}
	s.handleLoad(protocol.Request{ID: "load"})
	if !s.loaded || bytes.Contains(output.Bytes(), []byte(`"type":"error"`)) {
		t.Fatalf("unexpected load state: %s", output.String())
	}
	output.Reset()
	s.speak(context.Background(), protocol.Request{ID: "speak", Text: "Hello."})
	if !bytes.Contains(output.Bytes(), []byte(`"type":"audio_chunk"`)) {
		t.Fatalf("TTS fixture failed: %s", output.String())
	}

}
