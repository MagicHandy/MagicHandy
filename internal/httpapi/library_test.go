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
)

func TestImportAndListPatterns(t *testing.T) {
	server := newTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sample.funscript")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	sample, err := os.ReadFile(filepath.Join("..", "funscript", "testdata", "sample.funscript"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
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
		t.Fatalf("import status = %d, want 200: %s", importRecorder.Code, importRecorder.Body.String())
	}

	var importBody struct {
		OK        bool `json:"ok"`
		Persisted struct {
			FileID string `json:"file_id"`
		} `json:"persisted"`
	}
	if err := json.Unmarshal(importRecorder.Body.Bytes(), &importBody); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if !importBody.OK || importBody.Persisted.FileID == "" {
		t.Fatalf("import body = %s", importRecorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/api/import?limit=10", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list imports status = %d", listRecorder.Code)
	}

	patternsRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(patternsRecorder, httptest.NewRequest(http.MethodGet, "/api/patterns?limit=10", nil))
	if patternsRecorder.Code != http.StatusOK {
		t.Fatalf("patterns status = %d: %s", patternsRecorder.Code, patternsRecorder.Body.String())
	}
	var patternsBody struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(patternsRecorder.Body.Bytes(), &patternsBody); err != nil {
		t.Fatalf("decode patterns: %v", err)
	}
	if patternsBody.Total < 1 {
		t.Fatalf("expected imported patterns, got total=%d", patternsBody.Total)
	}
}

func TestManualQueueDraftCRUD(t *testing.T) {
	server := newTestServer(t)

	sample, err := os.ReadFile(filepath.Join("..", "funscript", "testdata", "sample.funscript"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "sample.funscript")
	_, _ = part.Write(sample)
	_ = writer.Close()
	importRequest := httptest.NewRequest(http.MethodPost, "/api/import", body)
	importRequest.Header.Set("Content-Type", writer.FormDataContentType())
	importRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("import status = %d", importRecorder.Code)
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
	blockID := importBody.Persisted.InsertedBlockIDs[0]

	addRecorder := httptest.NewRecorder()
	addRequest := httptest.NewRequest(http.MethodPost, "/api/manual-queue/items", bytes.NewReader([]byte(
		`{"block_id":"`+blockID+`","duration_sec":5,"loop":false}`,
	)))
	server.Handler().ServeHTTP(addRecorder, addRequest)
	if addRecorder.Code != http.StatusOK {
		t.Fatalf("add queue item status = %d: %s", addRecorder.Code, addRecorder.Body.String())
	}

	playRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(playRecorder, httptest.NewRequest(http.MethodPost, "/api/manual-queue/play", nil))
	if playRecorder.Code != http.StatusOK {
		t.Fatalf("play queue status = %d: %s", playRecorder.Code, playRecorder.Body.String())
	}
}
