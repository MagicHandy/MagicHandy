package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/media"
)

func TestMediaScanCatalogAndRangeStreaming(t *testing.T) {
	server := newTestServer(t)
	root := t.TempDir()
	content := "0123456789"
	if err := os.WriteFile(filepath.Join(root, "Range sample.mp4"), []byte(content), 0o600); err != nil {
		t.Fatalf("write video: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Range sample.funscript"), []byte(`{"actions":[]}`), 0o600); err != nil {
		t.Fatalf("write funscript: %v", err)
	}
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Media.LibraryPaths = []string{root}
		return settings
	})

	start := httptest.NewRecorder()
	server.Handler().ServeHTTP(start, withController(httptest.NewRequest(http.MethodPost, "/api/media/scan", strings.NewReader(`{}`))))
	if start.Code != http.StatusAccepted {
		t.Fatalf("scan start status = %d: %s", start.Code, start.Body.String())
	}
	waitForMediaScan(t, server)

	list := httptest.NewRecorder()
	server.Handler().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/media/videos", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body.String())
	}
	var payload struct {
		Videos []media.Video `json:"videos"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(payload.Videos) != 1 || !payload.Videos[0].HasFunscript {
		t.Fatalf("videos = %+v", payload.Videos)
	}
	if strings.Contains(list.Body.String(), "relative_path") || strings.Contains(list.Body.String(), ".funscript") {
		t.Fatalf("catalog API leaked jailed relative paths: %s", list.Body.String())
	}

	rangeResponse := httptest.NewRecorder()
	rangeRequest := httptest.NewRequest(http.MethodGet, "/api/media/videos/"+payload.Videos[0].ID+"/stream", nil)
	rangeRequest.Header.Set("Range", "bytes=2-5")
	server.Handler().ServeHTTP(rangeResponse, rangeRequest)
	if rangeResponse.Code != http.StatusPartialContent || rangeResponse.Body.String() != "2345" {
		t.Fatalf("range status=%d body=%q headers=%v", rangeResponse.Code, rangeResponse.Body.String(), rangeResponse.Header())
	}
	if rangeResponse.Header().Get("Accept-Ranges") != "bytes" || rangeResponse.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("range headers = %v", rangeResponse.Header())
	}

	duration := httptest.NewRecorder()
	durationBody := `{"id":"` + payload.Videos[0].ID + `","duration_ms":42000}`
	server.Handler().ServeHTTP(duration, withController(httptest.NewRequest(http.MethodPost, "/api/media/duration", strings.NewReader(durationBody))))
	if duration.Code != http.StatusOK {
		t.Fatalf("duration status = %d: %s", duration.Code, duration.Body.String())
	}
}

func TestMediaRoutesGateWritesAndRejectUnknownStreamIDs(t *testing.T) {
	server := newTestServer(t)
	for _, testCase := range []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{method: http.MethodPost, path: "/api/media/scan", body: `{}`, want: http.StatusConflict},
		{method: http.MethodDelete, path: "/api/media/scan", want: http.StatusConflict},
		{method: http.MethodPost, path: "/api/media/duration", body: `{"id":"missing","duration_ms":1}`, want: http.StatusConflict},
		{method: http.MethodGet, path: "/api/media/videos/not-a-catalog-id/stream", want: http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body)))
		if recorder.Code != testCase.want {
			t.Errorf("%s %s status = %d, want %d: %s", testCase.method, testCase.path, recorder.Code, testCase.want, recorder.Body.String())
		}
	}
}

func TestSavedMediaLocationRemovalPrunesCatalog(t *testing.T) {
	server := newTestServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "remove.mp4"), []byte("video"), 0o600); err != nil {
		t.Fatalf("write video: %v", err)
	}
	previous, _ := server.store.Snapshot()
	withRoot := previous
	withRoot.Media.LibraryPaths = []string{root}
	if _, err := server.store.Save(withRoot); err != nil {
		t.Fatalf("save media settings: %v", err)
	}
	if _, err := server.media.StartScan([]string{root}); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForMediaScan(t, server)

	withoutRoot := withRoot
	withoutRoot.Media.LibraryPaths = nil
	if err := server.applySettingsRuntimeTransition(t.Context(), withRoot, withoutRoot); err != nil {
		t.Fatalf("apply settings transition: %v", err)
	}
	videos, err := server.media.List(t.Context())
	if err != nil || len(videos) != 0 {
		t.Fatalf("catalog after removal: videos=%+v err=%v", videos, err)
	}
}

func waitForMediaScan(t *testing.T, server *Server) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !server.media.ScanState().Running {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("media scan did not finish")
}
