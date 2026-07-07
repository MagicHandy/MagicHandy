package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPatternsMetaReturnsLSOShape(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/patterns/meta", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var body struct {
		Zones []map[string]string `json:"zones"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Zones) == 0 {
		t.Fatal("expected zones metadata")
	}
}

func TestManualQueueDraftRoundTrip(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/manual-queue", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var draft struct {
		Items []any `json:"items"`
		Count int   `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if draft.Count != 0 || len(draft.Items) != 0 {
		t.Fatalf("empty draft = %+v, want zero items", draft)
	}
}

func TestImportUploadPersistsFunscript(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	samplePath := filepath.Join("..", "funscript", "testdata", "sample.funscript")
	sample, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sample.funscript")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(sample)); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/import", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var result struct {
		OK        bool `json:"ok"`
		Persisted struct {
			FileID string `json:"file_id"`
		} `json:"persisted"`
		Blocks []map[string]any `json:"blocks"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if !result.OK {
		t.Fatal("expected ok=true")
	}
	if result.Persisted.FileID == "" {
		t.Fatal("expected persisted file_id")
	}
	if len(result.Blocks) == 0 {
		t.Fatal("expected at least one block")
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/import?limit=10", nil)
	server.Handler().ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRecorder.Code, http.StatusOK)
	}
}
