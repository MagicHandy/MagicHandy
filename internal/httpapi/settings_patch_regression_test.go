package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestStaleDiagnosticsDraftPreservesNewerMotionLimits(t *testing.T) {
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(s.Close)
	callMotion(t, s, http.MethodPost, "/api/motion/quick", `{"stroke_min_percent":30,"stroke_max_percent":65}`)
	save := func(body string) int {
		r := withController(httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(body)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if code := save(`{"changes":{"diagnostics":{"verbosity":"debug"}},"expected":{"diagnostics":{"verbosity":"normal"}}}`); code != 200 {
		t.Fatalf("save %d", code)
	}
	if code := save(`{"changes":{"diagnostics":{"verbosity":"trace"},"ui":{"locale":"ja"}},"expected":{"diagnostics":{"verbosity":"normal"},"ui":{"locale":"en"}}}`); code != 409 {
		t.Fatalf("conflicting save %d", code)
	}
	settings, _ := s.store.Snapshot()
	if settings.Motion.StrokeMinPercent != 30 || settings.Motion.StrokeMaxPercent != 65 || settings.Diagnostics.Verbosity != "debug" || settings.UI.Locale != "en" {
		t.Fatal("an unrelated or conflicting edit overwrote persisted fields")
	}
}

func TestWriteOnlySettingsEditsRequireCurrentRevision(t *testing.T) {
	s := newTestServer(t)
	_, initial := s.store.Snapshot()
	secretPatch := func(revision, value string, wasSet bool) string {
		return fmt.Sprintf(`{"credential_revision":%q,"changes":{"voice":{"elevenlabs_api_key":%q}},"expected":{"voice":{"elevenlabs_api_key":%t}}}`, revision, value, wasSet)
	}
	for _, step := range []struct {
		body string
		want int
	}{
		{secretPatch(initial.LoadedAt, "fixture-first", false), http.StatusOK},
		{`{"changes":{"diagnostics":{"verbosity":"debug"}},"expected":{"diagnostics":{"verbosity":"normal"}}}`, http.StatusOK},
		{secretPatch(initial.LoadedAt, "fixture-stale", true), http.StatusConflict},
	} {
		res := handyAPI(t, s, http.MethodPatch, "/api/settings", step.body)
		if res.Code != step.want || strings.Contains(res.Body.String(), "fixture-") {
			t.Fatalf("patch status %d, want %d, or response revealed write-only field", res.Code, step.want)
		}
	}
	settings, latest := s.store.Snapshot()
	if settings.Voice.ElevenLabsAPIKey != "fixture-first" {
		t.Fatal("unrelated or stale edit changed stored credential")
	}
	res := handyAPI(t, s, http.MethodPatch, "/api/settings", secretPatch(latest.LoadedAt, "fixture-latest", true))
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "fixture-") {
		t.Fatalf("current edit returned %d or revealed credential", res.Code)
	}
}
