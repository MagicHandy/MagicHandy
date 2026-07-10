package uibuild

import (
	"io/fs"
	"strings"
	"testing"
)

// The UI is the Vite/React frontend build copied into uibuild/dist.
// These tests assert the generated app shell and safety-relevant strings survive the build.

func TestEmbeddedAppShellBuilt(t *testing.T) {
	ui, err := FS()
	if err != nil {
		t.Fatalf("uibuild.FS(): %v", err)
	}
	index, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		t.Fatalf("built index.html missing (run `npm run build` in frontend/ and copy to uibuild/dist): %v", err)
	}
	for _, fragment := range []string{`id="root"`, `/assets/`, `type="module"`} {
		if !strings.Contains(string(index), fragment) {
			t.Fatalf("built index.html missing %q", fragment)
		}
	}
}

func TestEmbeddedCriticalHooksSurviveBuild(t *testing.T) {
	ui, err := FS()
	if err != nil {
		t.Fatalf("uibuild.FS(): %v", err)
	}
	js := readBuiltJS(t, ui)
	for _, fragment := range []string{
		"Parar",
		"Stop",
		"emergency-stop",
		"motion/stop",
		"controller",
		"X-MagicHandy-Client-ID",
		"Somente leitura",
		"Read-only",
		"MagicHandy",
		"Controle",
	} {
		if !strings.Contains(js, fragment) {
			t.Fatalf("built bundle missing critical string %q", fragment)
		}
	}
}

func readBuiltJS(t *testing.T, ui fs.FS) string {
	t.Helper()
	entries, err := fs.ReadDir(ui, "assets")
	if err != nil {
		t.Fatalf("built assets/ dir missing: %v", err)
	}
	var combined strings.Builder
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".js") {
			data, err := fs.ReadFile(ui, "assets/"+e.Name())
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
