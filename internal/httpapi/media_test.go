package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/media"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestMediaScanCatalogAndRangeStreaming(t *testing.T) {
	server := newTestServer(t)
	root := t.TempDir()
	content := "0123456789"
	if err := os.WriteFile(filepath.Join(root, "Range sample.mp4"), []byte(content), 0o600); err != nil {
		t.Fatalf("write video: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Range sample.funscript"), []byte(`{"actions":[{"at":0,"pos":10},{"at":1000,"pos":90}]}`), 0o600); err != nil {
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
	if rangeResponse.Header().Get("Content-Type") != "video/mp4" || rangeResponse.Header().Get("Cache-Control") != "no-store" || rangeResponse.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("stream safety headers = %v", rangeResponse.Header())
	}

	duration := httptest.NewRecorder()
	durationBody := `{"id":"` + payload.Videos[0].ID + `","duration_ms":42000}`
	server.Handler().ServeHTTP(duration, withController(httptest.NewRequest(http.MethodPost, "/api/media/duration", strings.NewReader(durationBody))))
	if duration.Code != http.StatusOK {
		t.Fatalf("duration status = %d: %s", duration.Code, duration.Body.String())
	}

	assertMediaFunscriptResponse(t, server, payload.Videos[0].ID, root)
}

func assertMediaFunscriptResponse(t *testing.T, server *Server, videoID, root string) {
	t.Helper()
	funscript := httptest.NewRecorder()
	server.Handler().ServeHTTP(funscript, httptest.NewRequest(http.MethodGet, "/api/media/videos/"+videoID+"/funscript", nil))
	if funscript.Code != http.StatusOK || !strings.Contains(funscript.Body.String(), `"action_count":2`) {
		t.Fatalf("funscript status = %d: %s", funscript.Code, funscript.Body.String())
	}
	if strings.Contains(funscript.Body.String(), root) || strings.Contains(funscript.Body.String(), ".funscript") {
		t.Fatalf("funscript API leaked a filesystem path: %s", funscript.Body.String())
	}
}

func TestMediaSyncDrivesPairedTimelineThroughSharedEngine(t *testing.T) {
	server := newTestServer(t)
	fake := server.transport.(*transport.Fake)
	root := t.TempDir()
	writeMediaPair(t, root, "Session", `{"actions":[{"at":0,"pos":0},{"at":500,"pos":100},{"at":1000,"pos":0},{"at":3000,"pos":100}]}`)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Media.LibraryPaths = []string{root}
		settings.Motion.SpeedMaxPercent = 40
		return settings
	})
	if _, err := server.media.StartScan([]string{root}); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForMediaScan(t, server)
	video := mustSingleMediaVideo(t, server)
	sequence := server.stopSequence.Load()

	play := postMediaSync(t, server, sequence, video.ID, "playing", "play", 0, 1)
	if play.Code != http.StatusOK || !strings.Contains(play.Body.String(), `"state":"following"`) {
		t.Fatalf("play status = %d: %s", play.Code, play.Body.String())
	}
	engine := server.currentMotionEngine().Snapshot()
	if !engine.Running || engine.Target.Source != motion.TargetSourceMedia || engine.Target.MediaID != video.ID || engine.Target.SpeedPercent != 40 {
		t.Fatalf("media engine = %+v", engine)
	}
	if got := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay); got != 1 {
		t.Fatalf("play commands = %d, want 1", got)
	}

	heartbeat := postMediaSync(t, server, sequence, video.ID, "playing", "heartbeat", 0, 1)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d: %s", heartbeat.Code, heartbeat.Body.String())
	}
	if got := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay); got != 1 {
		t.Fatalf("steady heartbeat restarted playback: %d play commands", got)
	}

	seeking := postMediaSync(t, server, sequence, video.ID, "seeking", "seeking", 750, 1)
	if seeking.Code != http.StatusOK || !strings.Contains(seeking.Body.String(), `"state":"seeking"`) {
		t.Fatalf("seeking status = %d: %s", seeking.Code, seeking.Body.String())
	}
	resumed := postMediaSync(t, server, sequence, video.ID, "playing", "seeked", 750, 1)
	if resumed.Code != http.StatusOK {
		t.Fatalf("seeked status = %d: %s", resumed.Code, resumed.Body.String())
	}
	if got := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay); got != 2 {
		t.Fatalf("play commands after seek = %d, want 2", got)
	}

	paused := postMediaSync(t, server, sequence, video.ID, "paused", "pause", 800, 1)
	if paused.Code != http.StatusOK || server.currentMotionEngine().Snapshot().Running {
		t.Fatalf("pause status = %d engine=%+v", paused.Code, server.currentMotionEngine().Snapshot())
	}
	_ = postMediaSync(t, server, sequence, video.ID, "playing", "play", 800, 1)
	ended := postMediaSync(t, server, sequence, video.ID, "ended", "ended", 3000, 1)
	if ended.Code != http.StatusOK || server.currentMotionEngine().Snapshot().Running {
		t.Fatalf("ended status = %d engine=%+v", ended.Code, server.currentMotionEngine().Snapshot())
	}
}

func TestMediaSyncReturnsSafeMotionStartupError(t *testing.T) {
	owner := &mediaStartupFailureTransport{Fake: transport.NewFake()}
	server := newTestServerWithRuntime(t, Runtime{Transport: owner, MotionTransport: owner})
	root := t.TempDir()
	writeMediaPair(t, root, "Unavailable", `{"actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`)
	if _, err := server.media.StartScan([]string{root}); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForMediaScan(t, server)
	video := mustSingleMediaVideo(t, server)

	play := postMediaSync(t, server, server.stopSequence.Load(), video.ID, "playing", "play", 0, 1)
	if play.Code != http.StatusBadGateway || !strings.Contains(play.Body.String(), "startup state unavailable") {
		t.Fatalf("play status = %d: %s", play.Code, play.Body.String())
	}
}

type mediaStartupFailureTransport struct {
	*transport.Fake
}

func (m *mediaStartupFailureTransport) ReadMotionStartupState(context.Context) (
	transport.MotionStartupState,
	transport.MotionStartupStateResults,
	error,
) {
	result := transport.CommandResult{
		Kind:      transport.CommandKindSliderState,
		Transport: "startup_failure",
		Status:    "failed",
	}
	return transport.MotionStartupState{}, transport.MotionStartupStateResults{Slider: result}, errors.New("startup state unavailable")
}

func TestMediaSyncHeartbeatCannotRestartAfterEmergencyStopAndTimeoutStops(t *testing.T) {
	server := newTestServer(t)
	fake := server.transport.(*transport.Fake)
	root := t.TempDir()
	writeMediaPair(t, root, "Guarded", `{"actions":[{"at":0,"pos":0},{"at":10000,"pos":100}]}`)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Media.LibraryPaths = []string{root}
		return settings
	})
	if _, err := server.media.StartScan([]string{root}); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForMediaScan(t, server)
	video := mustSingleMediaVideo(t, server)
	sequence := server.stopSequence.Load()
	if got := postMediaSync(t, server, sequence, video.ID, "playing", "play", 0, 1); got.Code != http.StatusOK {
		t.Fatalf("play status = %d: %s", got.Code, got.Body.String())
	}

	server.mediaSync.expireHeartbeat(time.Now().UTC().Add(mediaHeartbeatTimeout + time.Second))
	if server.currentMotionEngine().Snapshot().Running || server.mediaSync.Status().State != "timed_out" {
		t.Fatalf("heartbeat expiry left media active: sync=%+v engine=%+v", server.mediaSync.Status(), server.currentMotionEngine().Snapshot())
	}
	if got := postMediaSync(t, server, sequence, video.ID, "playing", "play", 0, 1); got.Code != http.StatusOK {
		t.Fatalf("replay status = %d: %s", got.Code, got.Body.String())
	}
	playsBeforeStop := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay)
	stop := httptest.NewRecorder()
	server.Handler().ServeHTTP(stop, withController(httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`))))
	if stop.Code != http.StatusOK {
		t.Fatalf("stop status = %d: %s", stop.Code, stop.Body.String())
	}
	stale := postMediaSync(t, server, sequence, video.ID, "playing", "heartbeat", 100, 1)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale heartbeat status = %d: %s", stale.Code, stale.Body.String())
	}
	if got := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay); got != playsBeforeStop {
		t.Fatalf("stale heartbeat restarted motion: plays %d -> %d", playsBeforeStop, got)
	}
}

func TestMediaSyncDriftStopsAndRequiresExplicitRearm(t *testing.T) {
	server := newTestServer(t)
	fake := server.transport.(*transport.Fake)
	root := t.TempDir()
	writeMediaPair(t, root, "Drift", `{"actions":[{"at":0,"pos":0},{"at":10000,"pos":100}]}`)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Media.LibraryPaths = []string{root}
		return settings
	})
	if _, err := server.media.StartScan([]string{root}); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForMediaScan(t, server)
	video := mustSingleMediaVideo(t, server)
	sequence := server.stopSequence.Load()
	if got := postMediaSync(t, server, sequence, video.ID, "playing", "play", 0, 1); got.Code != http.StatusOK {
		t.Fatalf("play status = %d: %s", got.Code, got.Body.String())
	}

	server.mediaSync.mu.Lock()
	server.mediaSync.anchorAt = server.mediaSync.anchorAt.Add(-time.Second)
	server.mediaSync.mu.Unlock()
	drift := postMediaSync(t, server, sequence, video.ID, "playing", "heartbeat", 0, 1)
	if drift.Code != http.StatusOK || !strings.Contains(drift.Body.String(), `"requires_reanchor":true`) {
		t.Fatalf("drift status = %d: %s", drift.Code, drift.Body.String())
	}
	if server.currentMotionEngine().Snapshot().Running {
		t.Fatal("drift correction left media motion running")
	}
	playsAfterDrift := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay)
	staleHeartbeat := postMediaSync(t, server, sequence, video.ID, "playing", "heartbeat", 1000, 1)
	if staleHeartbeat.Code != http.StatusConflict {
		t.Fatalf("post-drift heartbeat status = %d: %s", staleHeartbeat.Code, staleHeartbeat.Body.String())
	}
	if got := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay); got != playsAfterDrift {
		t.Fatalf("post-drift heartbeat restarted motion: plays %d -> %d", playsAfterDrift, got)
	}

	resync := postMediaSync(t, server, sequence, video.ID, "playing", "resync", 1000, 1)
	if resync.Code != http.StatusOK || !server.currentMotionEngine().Snapshot().Running {
		t.Fatalf("resync status = %d engine=%+v", resync.Code, server.currentMotionEngine().Snapshot())
	}
	if got := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay); got != playsAfterDrift+1 {
		t.Fatalf("explicit resync play commands = %d, want %d", got, playsAfterDrift+1)
	}
}

func TestMediaSyncClosedSessionCannotRaceNewPlayback(t *testing.T) {
	server := newTestServer(t)
	fake := server.transport.(*transport.Fake)
	root := t.TempDir()
	writeMediaPair(t, root, "SessionFence", `{"actions":[{"at":0,"pos":0},{"at":10000,"pos":100}]}`)
	if _, err := server.media.StartScan([]string{root}); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForMediaScan(t, server)
	video := mustSingleMediaVideo(t, server)
	stopSequence := server.stopSequence.Load()

	closed := postMediaSyncForSession(t, server, stopSequence, video.ID, "old-player", 2, "closed", "closed", 0, 1)
	if closed.Code != http.StatusOK {
		t.Fatalf("close status = %d: %s", closed.Code, closed.Body.String())
	}
	delayedArm := postMediaSyncForSession(t, server, stopSequence, video.ID, "old-player", 1, "playing", "play", 0, 1)
	if delayedArm.Code != http.StatusOK || server.currentMotionEngine() != nil {
		t.Fatalf("delayed arm status = %d engine=%+v", delayedArm.Code, server.currentMotionEngine())
	}

	newArm := postMediaSyncForSession(t, server, stopSequence, video.ID, "new-player", 1, "playing", "play", 0, 1)
	if newArm.Code != http.StatusOK || !server.currentMotionEngine().Snapshot().Running {
		t.Fatalf("new arm status = %d engine=%+v", newArm.Code, server.currentMotionEngine())
	}
	oldPause := postMediaSyncForSession(t, server, stopSequence, video.ID, "old-player", 3, "paused", "pause", 100, 1)
	if oldPause.Code != http.StatusOK || !server.currentMotionEngine().Snapshot().Running {
		t.Fatalf("old pause status = %d engine=%+v", oldPause.Code, server.currentMotionEngine().Snapshot())
	}
	if got := countTransportCommands(fake.Commands(), transport.CommandKindPointsPlay); got != 1 {
		t.Fatalf("play commands = %d, want only the new session", got)
	}
}

func TestMediaContentTypeDoesNotDependOnHostRegistry(t *testing.T) {
	for _, testCase := range []struct {
		path string
		want string
	}{
		{path: "sample.mp4", want: "video/mp4"},
		{path: "sample.m4v", want: "video/mp4"},
		{path: "sample.webm", want: "video/webm"},
		{path: "sample.mov", want: "video/quicktime"},
	} {
		t.Run(filepath.Ext(testCase.path), func(t *testing.T) {
			if got := mediaContentType(testCase.path); got != testCase.want {
				t.Fatalf("mediaContentType(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
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
		{method: http.MethodPost, path: "/api/media/sync", body: `{"video_id":"missing","state":"playing","event":"play","media_time_ms":0,"playback_rate":1}`, want: http.StatusConflict},
		{method: http.MethodGet, path: "/api/media/videos/not-a-catalog-id/stream", want: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/media/videos/not-a-catalog-id/funscript", want: http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body)))
		if recorder.Code != testCase.want {
			t.Errorf("%s %s status = %d, want %d: %s", testCase.method, testCase.path, recorder.Code, testCase.want, recorder.Body.String())
		}
	}
	invalidSession := httptest.NewRecorder()
	request := withController(httptest.NewRequest(
		http.MethodPost,
		"/api/media/sync",
		strings.NewReader(`{"video_id":"missing","state":"playing","event":"play","media_time_ms":0,"playback_rate":1}`),
	))
	server.Handler().ServeHTTP(invalidSession, request)
	if invalidSession.Code != http.StatusBadRequest {
		t.Fatalf("media sync without session status = %d: %s", invalidSession.Code, invalidSession.Body.String())
	}
}

func writeMediaPair(t *testing.T, root, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name+".mp4"), []byte("video"), 0o600); err != nil {
		t.Fatalf("write video: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, name+".funscript"), []byte(script), 0o600); err != nil {
		t.Fatalf("write funscript: %v", err)
	}
}

func mustSingleMediaVideo(t *testing.T, server *Server) media.Video {
	t.Helper()
	videos, err := server.media.List(t.Context())
	if err != nil || len(videos) != 1 {
		t.Fatalf("media videos = %+v err=%v", videos, err)
	}
	return videos[0]
}

func postMediaSync(t *testing.T, server *Server, sequence uint64, videoID, state, event string, at int64, rate float64) *httptest.ResponseRecorder {
	t.Helper()
	const sessionID = "test-player"
	server.mediaSync.mu.Lock()
	eventSequence := server.mediaSync.fences[sessionID].Sequence + 1
	server.mediaSync.mu.Unlock()
	return postMediaSyncForSession(t, server, sequence, videoID, sessionID, eventSequence, state, event, at, rate)
}

func postMediaSyncForSession(t *testing.T, server *Server, stopSequence uint64, videoID, sessionID string, eventSequence uint64, state, event string, at int64, rate float64) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"video_id":%q,"session_id":%q,"event_sequence":%d,"state":%q,"event":%q,"media_time_ms":%d,"client_time_ms":1,"playback_rate":%g}`, videoID, sessionID, eventSequence, state, event, at, rate)
	request := withController(httptest.NewRequest(http.MethodPost, "/api/media/sync", strings.NewReader(body)))
	request.Header.Set(stopSequenceHeader, fmt.Sprint(stopSequence))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func countTransportCommands(commands []transport.Command, kind transport.CommandKind) int {
	count := 0
	for _, command := range commands {
		if command.Kind == kind {
			count++
		}
	}
	return count
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
