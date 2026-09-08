package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVoiceCandidateRequiresVerifiedChildAndExplicitActivation(t *testing.T) {
	home := t.TempDir()
	candidate := filepath.Join(home, "runtimes", "0123456789abcdef0123456789abcdef")
	if err := os.MkdirAll(candidate, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "module-state.json"), []byte(`{"schema_version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := json.Marshal(map[string]any{"schema_version": 2, "runtime_root": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "candidate-state.json"), index, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := installedTTSRuntimeRoot(home); got != home {
		t.Fatalf("unactivated runtime used: %s", got)
	}
	if err := activateVoiceInstallCandidate(home, candidate); err != nil {
		t.Fatal(err)
	}
	if got := installedTTSRuntimeRoot(home); got != candidate {
		t.Fatalf("activated runtime not selected: %s", got)
	}
	if _, err := voiceRuntimeFromIndex(home, "candidate-state.json"); err == nil {
		t.Fatal("candidate was not consumed")
	}
}

func TestVoiceIndexRejectsPathOutsideModuleHome(t *testing.T) {
	home := t.TempDir()
	index, err := json.Marshal(map[string]any{"schema_version": 2, "runtime_root": filepath.Join(t.TempDir(), "0123456789abcdef0123456789abcdef")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "module-state.json"), index, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := installedTTSRuntimeRoot(home); got != home {
		t.Fatal("external path accepted")
	}
}
