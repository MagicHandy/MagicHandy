package openaittsworker

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

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
