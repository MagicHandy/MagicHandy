package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestManualQueuePlayDispatchesHSP(t *testing.T) {
	requests := make(chan capturedCloudRequest, 8)
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captureCloudRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"hsp_available":true,"playback_state":"buffered"}`))
	}))
	defer cloudServer.Close()

	server := newCloudTestServer(t, Runtime{CloudBaseURL: cloudServer.URL})
	saveCloudSettings(t, server)
	t.Cleanup(server.Close)

	blockID := importSampleBlock(t, server)

	addRecorder := httptest.NewRecorder()
	addRequest := httptest.NewRequest(http.MethodPost, "/api/manual-queue/items", bytes.NewReader([]byte(
		`{"block_id":"`+blockID+`","duration_sec":5,"loop":false}`,
	)))
	addRequest.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(addRecorder, addRequest)
	if addRecorder.Code != http.StatusOK {
		t.Fatalf("add queue item status = %d: %s", addRecorder.Code, addRecorder.Body.String())
	}

	playRecorder := httptest.NewRecorder()
	playRequest := withController(httptest.NewRequest(http.MethodPost, "/api/manual-queue/play", nil))
	server.Handler().ServeHTTP(playRecorder, playRequest)
	if playRecorder.Code != http.StatusOK {
		t.Fatalf("play queue status = %d: %s", playRecorder.Code, playRecorder.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	var sawSetup, sawAdd, sawPlay bool
	for time.Now().Before(deadline) {
		select {
		case req := <-requests:
			switch req.Path {
			case "/hsp/setup":
				sawSetup = true
			case "/hsp/add":
				sawAdd = true
			case "/hsp/play":
				sawPlay = true
			}
		default:
			time.Sleep(20 * time.Millisecond)
		}
		if sawSetup && sawAdd && sawPlay {
			break
		}
	}
	if !sawSetup || !sawAdd || !sawPlay {
		t.Fatalf("cloud dispatch = setup:%v add:%v play:%v", sawSetup, sawAdd, sawPlay)
	}

	stopRecorder := httptest.NewRecorder()
	stopRequest := withController(httptest.NewRequest(http.MethodPost, "/api/manual-queue/player/stop", nil))
	server.Handler().ServeHTTP(stopRecorder, stopRequest)
	if stopRecorder.Code != http.StatusOK {
		t.Fatalf("stop queue status = %d: %s", stopRecorder.Code, stopRecorder.Body.String())
	}
}

func importSampleBlock(t *testing.T, server *Server) string {
	t.Helper()
	sample, err := os.ReadFile(filepath.Join("..", "funscript", "testdata", "sample.funscript"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sample.funscript")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(sample); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}
	importRequest := httptest.NewRequest(http.MethodPost, "/api/import", body)
	importRequest.Header.Set("Content-Type", writer.FormDataContentType())
	importRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("import status = %d: %s", importRecorder.Code, importRecorder.Body.String())
	}
	var importBody struct {
		Persisted struct {
			InsertedBlockIDs []string `json:"inserted_block_ids"`
		} `json:"persisted"`
	}
	if err := json.Unmarshal(importRecorder.Body.Bytes(), &importBody); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if len(importBody.Persisted.InsertedBlockIDs) == 0 {
		t.Fatal("expected inserted block ids")
	}
	return importBody.Persisted.InsertedBlockIDs[0]
}

// Ensure cloud transport types stay linked for the test package.
var _ transport.CommandKind = transport.CommandKindHSPPlay
