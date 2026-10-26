package openaittsworker

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

func TestInterruptedManagedSynthesisRequiresFreshRuntime(t *testing.T) {
	for _, managed := range []bool{false, true} {
		var output bytes.Buffer
		s := &session{writer: &output, loaded: true}
		if managed {
			s.server = &managedServer{}
		}
		s.jobs.Track("speech")
		s.streamAudio(t.Context(), "speech", io.MultiReader(bytes.NewReader(streamingTestWAV()), iotest.ErrReader(io.ErrUnexpectedEOF)), "wav")
		if s.modelLoaded() == managed || (s.managedFailureMessage() != "") != managed {
			t.Fatalf("managed=%v loaded=%v failure=%q", managed, s.modelLoaded(), s.managedFailureMessage())
		}
		if !bytes.Contains(output.Bytes(), []byte(`"type":"error"`)) || bytes.Contains(output.Bytes(), []byte(`"type":"done"`)) {
			t.Fatalf("partial audio looked complete: %s", output.Bytes())
		}
		s.jobs.Finish("speech")
	}
}

func TestCanceledSynthesisDoesNotPoisonManagedReadiness(t *testing.T) {
	var output bytes.Buffer
	s := &session{writer: &output, loaded: true, server: &managedServer{}}
	s.jobs.Track("speech")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.streamAudio(ctx, "speech", bytes.NewReader(streamingTestWAV()), "wav")
	if !s.modelLoaded() || s.managedFailureMessage() != "" {
		t.Fatal("intentional cancellation made model fail")
	}
	if !bytes.Contains(output.Bytes(), []byte(`"type":"canceled"`)) {
		t.Fatalf("expected cancellation, got %s", output.Bytes())
	}
	s.jobs.Finish("speech")
}

func TestFirstAudioFrameArrivesBeforeHTTPResponseCompletes(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(streamingTestWAV())
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	driver := loadWorker(t, server.URL)
	t.Cleanup(func() { close(release) })
	driver.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "stream", Text: "Hello."})
	first := driver.next(t, time.Second)
	if first.Type != protocol.ResponseAudioChunk {
		t.Fatalf("first response: %+v", first)
	}
}
