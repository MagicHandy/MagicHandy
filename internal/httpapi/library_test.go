package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLibraryStorageFailureIsNotReportedAsEmptyOrDisabled(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Must not run.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	if err := server.patterns.Close(); err != nil {
		t.Fatal(err)
	}

	list := httptest.NewRecorder()
	server.Handler().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/library", nil))
	if list.Code != http.StatusInternalServerError ||
		!strings.Contains(list.Body.String(), "pattern library storage is unavailable") {
		t.Fatalf("library read = %d: %s", list.Code, list.Body.String())
	}

	state := httptest.NewRecorder()
	server.Handler().ServeHTTP(state, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if state.Code != http.StatusOK || !strings.Contains(state.Body.String(), `"library":{"available":false`) {
		t.Fatalf("state with failed library = %d: %s", state.Code, state.Body.String())
	}

	play := httptest.NewRecorder()
	playRequest := withController(httptest.NewRequest(
		http.MethodPost,
		"/api/library/patterns/stroke/play",
		strings.NewReader(`{"intensity":20}`),
	))
	server.Handler().ServeHTTP(play, playRequest)
	if play.Code != http.StatusInternalServerError {
		t.Fatalf("play with failed library = %d: %s", play.Code, play.Body.String())
	}

	chatRecorder := httptest.NewRecorder()
	chatRequest := withController(httptest.NewRequest(
		http.MethodPost,
		"/api/chat/stream",
		strings.NewReader(`{"message":"hello"}`),
	))
	chatRequest.Header.Set("Content-Type", "application/json")
	chatRequest.Header.Set(stopSequenceHeader, strconv.FormatUint(server.stopSequence.Load(), 10))
	server.Handler().ServeHTTP(chatRecorder, chatRequest)
	if chatRecorder.Code != http.StatusInternalServerError || provider.callCount() != 0 {
		t.Fatalf("chat with failed library = %d, provider calls = %d: %s",
			chatRecorder.Code, provider.callCount(), chatRecorder.Body.String())
	}
}

func TestLibraryAPIEnablementAndPlaybackUseMotionEngine(t *testing.T) {
	server := newTestServer(t)
	list := httptest.NewRecorder()
	server.Handler().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/library", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("library list = %d: %s", list.Code, list.Body.String())
	}
	var snapshot struct {
		Library struct {
			Patterns []struct {
				ID      string `json:"id"`
				Enabled bool   `json:"enabled"`
			} `json:"patterns"`
		} `json:"library"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Library.Patterns) < 3 {
		t.Fatalf("seeded library = %+v", snapshot.Library.Patterns)
	}
	id := snapshot.Library.Patterns[0].ID

	patch := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPatch, "/api/library/patterns/"+id, strings.NewReader(`{"enabled":false}`)))
	server.Handler().ServeHTTP(patch, request)
	if patch.Code != http.StatusOK {
		t.Fatalf("disable pattern = %d: %s", patch.Code, patch.Body.String())
	}
	disabledPlay := httptest.NewRecorder()
	server.Handler().ServeHTTP(disabledPlay, withController(httptest.NewRequest(http.MethodPost, "/api/library/patterns/"+id+"/play", strings.NewReader(`{"intensity":30}`))))
	if disabledPlay.Code != http.StatusConflict {
		t.Fatalf("disabled play = %d: %s", disabledPlay.Code, disabledPlay.Body.String())
	}

	enable := httptest.NewRecorder()
	server.Handler().ServeHTTP(enable, withController(httptest.NewRequest(http.MethodPatch, "/api/library/patterns/"+id, strings.NewReader(`{"enabled":true}`))))
	play := httptest.NewRecorder()
	server.Handler().ServeHTTP(play, withController(httptest.NewRequest(http.MethodPost, "/api/library/patterns/"+id+"/play", strings.NewReader(`{"intensity":30,"feel":"smooth"}`))))
	if play.Code != http.StatusOK {
		t.Fatalf("play pattern = %d: %s", play.Code, play.Body.String())
	}
	engine := server.currentMotionEngine()
	if engine == nil || !engine.Snapshot().Running || string(engine.Snapshot().Target.PatternID) != id {
		t.Fatalf("engine did not own pattern playback: %+v", engine)
	}
}

func TestLibraryFunscriptProgramCompletesAndPreviewIsBackendSampled(t *testing.T) {
	server := newTestServer(t)
	previewBody := `{"name":"Drawn","kind":"routine","cycle_ms":6600,"simplify_error":1,"points":[{"time_ms":0,"position_percent":0},{"time_ms":3300,"position_percent":100},{"time_ms":6600,"position_percent":0}]}`
	preview := httptest.NewRecorder()
	server.Handler().ServeHTTP(preview, withController(httptest.NewRequest(http.MethodPost, "/api/library/preview", strings.NewReader(previewBody))))
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"samples"`) {
		t.Fatalf("preview = %d: %s", preview.Code, preview.Body.String())
	}

	funscript := `{"actions":[{"at":0,"pos":0},{"at":500,"pos":100}]}`
	imported := httptest.NewRecorder()
	server.Handler().ServeHTTP(imported, withController(httptest.NewRequest(http.MethodPost, "/api/library/import?filename=short.funscript&as=program", strings.NewReader(funscript))))
	if imported.Code != http.StatusCreated {
		t.Fatalf("import = %d: %s", imported.Code, imported.Body.String())
	}
	var result struct {
		Import struct {
			Program struct {
				ID string `json:"id"`
			} `json:"program"`
		} `json:"import"`
	}
	if err := json.Unmarshal(imported.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	play := httptest.NewRecorder()
	server.Handler().ServeHTTP(play, withController(httptest.NewRequest(http.MethodPost, "/api/library/programs/"+result.Import.Program.ID+"/play", strings.NewReader(`{"intensity":80}`))))
	if play.Code != http.StatusOK {
		t.Fatalf("program play = %d: %s", play.Code, play.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for state := server.currentMotionEngine().Snapshot(); (state.Running || state.Completing) && time.Now().Before(deadline); state = server.currentMotionEngine().Snapshot() {
		time.Sleep(20 * time.Millisecond)
	}
	state := server.currentMotionEngine().Snapshot()
	if state.Running || state.Completing || state.Target.ProgramID != result.Import.Program.ID {
		t.Fatalf("finite program state = %+v", state)
	}
	if state.Phase != 1 {
		t.Fatalf("finite program phase = %.3f, want complete", state.Phase)
	}
}
