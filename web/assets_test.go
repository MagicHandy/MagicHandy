package web

import (
	"io/fs"
	"strings"
	"testing"
)

// The UI is now a built Vite/React bundle. These tests assert the generated app
// shell and that critical, safety-relevant strings survive the build, per
// docs/decisions/0009-react-frontend.md. Behavioral coverage of the shell lives
// in the Vitest component tests (web/src/**/*.test.tsx).

func TestEmbeddedAppShellBuilt(t *testing.T) {
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("built index.html missing (run `npm run build` in web/): %v", err)
	}
	for _, fragment := range []string{`id="root"`, `/assets/`, `type="module"`} {
		if !strings.Contains(string(index), fragment) {
			t.Fatalf("built index.html missing %q", fragment)
		}
	}
}

func TestEmbeddedCriticalHooksSurviveBuild(t *testing.T) {
	js := readBuiltJS(t)
	// Safety-critical and shell wiring must be present in the shipped bundle:
	// the permanent Stop and its endpoint, the four routes, the compact status
	// language, chat/motion endpoints, the read-only lock, and the honest
	// commanded-estimate label.
	for _, fragment := range []string{
		"Stop everything",
		"/api/motion/stop",
		"/api/chat/stream",
		"/api/motion/events",
		"Preset modes",
		"Pattern library",
		"Autopilot",
		"Commanded position estimate",
		"read-only",
		"controller: you",
		"#/settings",
		"X-MagicHandy-Client-ID",
		// Voice worker status UI (Phase 12): the optional-worker states must
		// stay visible readouts, and the API paths must survive minification.
		"Voice workers",
		"/api/voice/status",
		"/api/voice/workers/",
		"not configured",
	} {
		if !strings.Contains(js, fragment) {
			t.Fatalf("built bundle missing critical string %q", fragment)
		}
	}
	// The old oversized-round-bubble status class must not come back.
	if strings.Contains(js, "status-pill") {
		t.Fatal("built bundle contains status-pill; status readouts must be compact dot+text, not round pills")
	}
}

func readBuiltJS(t *testing.T) string {
	t.Helper()
	entries, err := fs.ReadDir(FS(), "assets")
	if err != nil {
		t.Fatalf("built assets/ dir missing (run `npm run build` in web/): %v", err)
	}
	var combined strings.Builder
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".js") {
			data, err := fs.ReadFile(FS(), "assets/"+e.Name())
			if err != nil {
				t.Fatalf("read built asset %s: %v", e.Name(), err)
			}
			combined.Write(data)
			found = true
		}
	}
	if !found {
		t.Fatal("no built JS asset found under dist/assets")
	}
	return combined.String()
}
