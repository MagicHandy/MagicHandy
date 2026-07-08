package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMotionSyncOffsetPersistAndVisual(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	put := httptest.NewRequest(http.MethodPut, "/api/motion/sync-offset", strings.NewReader(`{"offset_ms":-320}`))
	put.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, put)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT sync-offset status = %d, body = %s", putRec.Code, putRec.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/motion/visual", nil)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, get)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET visual status = %d", getRec.Code)
	}
	var visual map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &visual); err != nil {
		t.Fatalf("decode visual: %v", err)
	}
	if offset, _ := visual["offset_ms"].(float64); int(offset) != -320 {
		t.Fatalf("offset_ms = %v, want -320", visual["offset_ms"])
	}
	if _, ok := visual["recent"].([]any); !ok {
		t.Fatalf("recent missing or wrong type: %T", visual["recent"])
	}
}

func TestMotionAutoSync(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := httptest.NewRequest(http.MethodPost, "/api/motion/auto-sync", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST auto-sync status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode auto-sync: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %+v", body)
	}
	offset, _ := body["offset_ms"].(float64)
	if offset >= 0 {
		t.Fatalf("offset_ms = %v, want negative", offset)
	}
}
