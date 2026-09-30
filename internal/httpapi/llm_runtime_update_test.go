package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// writeOutdatedManagedRuntime installs a runtime manifest from an older pin.
func writeOutdatedManagedRuntime(t *testing.T, dataDir, backend string) {
	t.Helper()
	root := llm.ManagedLlamaRuntimeRoot(dataDir)
	runnerRelative := "installs/b9966-" + backend + "-c749cb0/bin/llama-server.exe"
	runner := filepath.Join(root, filepath.FromSlash(runnerRelative))
	if err := os.MkdirAll(filepath.Dir(runner), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runner, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"schema_version": 1, "runtime": "llama.cpp", "version": "b9966",
		"commit": "c749cb041706647f460bb918cccc9d91995205ab", "backend": backend,
		"runner": runnerRelative, "source": "verified_upstream_release",
		"built_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "active.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManagedRuntimeUpdatesFollowTheUsersChoice(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("managed runtime installs are Windows/amd64 only")
	}
	for name, test := range map[string]struct {
		mode      string
		completed bool
		provider  string
		want      bool
	}{
		"automatic":            {config.UpdateCheckAutomatic, true, config.LlamaCPPModeManaged, true},
		"manual only notifies": {config.UpdateCheckManual, true, config.LlamaCPPModeManaged, false},
		"setup still running":  {config.UpdateCheckAutomatic, false, config.LlamaCPPModeManaged, false},
		"external server":      {config.UpdateCheckAutomatic, true, config.LlamaCPPModeExternal, false},
	} {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t)
			var started []string
			server.runtimeUpdater = func(backend string) (llm.ManagedLlamaRuntimeBuild, error) {
				started = append(started, backend)
				return llm.ManagedLlamaRuntimeBuild{Status: llm.RuntimeBuildStatusQueued}, nil
			}
			writeOutdatedManagedRuntime(t, server.store.DataDir(), "cuda")
			saveSettings(t, server.store, func(settings config.Settings) config.Settings {
				settings.UI.RuntimeUpdateMode, settings.UI.SetupCompleted = test.mode, test.completed
				settings.LLM.Provider, settings.LLM.LlamaCPPMode = config.LLMProviderLlamaCPP, test.provider
				return settings
			})
			settings, _ := server.store.Snapshot()
			if got := server.startManagedRuntimeUpdate(settings); got != test.want {
				t.Fatalf("update started = %v, want %v", got, test.want)
			}
			if test.want && (len(started) != 1 || started[0] != "cuda") {
				t.Fatalf("updater calls = %v, want one with the installed CUDA backend", started)
			}
			if !test.want && len(started) != 0 {
				t.Fatalf("updater ran against the user's choice: %v", started)
			}
		})
	}
}

func TestLLMStateAnnouncesAnOutdatedRuntime(t *testing.T) {
	server := newTestServer(t)
	writeOutdatedManagedRuntime(t, server.store.DataDir(), "cpu")
	state, _ := server.llmState(context.Background()).(map[string]any)
	if state["managed_runtime"] != llm.ManagedRuntimeStateOutdated ||
		state["managed_runtime_version"] != "b9966" || state["managed_runtime_expected"] != llm.ManagedLlamaVersion {
		t.Fatalf("llm state = %+v", state)
	}
}

func TestRuntimeUpdateModeDefaultsToAutomaticAndValidates(t *testing.T) {
	settings := config.DefaultSettings()
	if settings.UI.RuntimeUpdateMode != config.UpdateCheckAutomatic {
		t.Fatalf("default runtime update mode = %q", settings.UI.RuntimeUpdateMode)
	}
	settings.UI.RuntimeUpdateMode = ""
	normalized, err := config.NormalizeSettings(settings)
	if err != nil || normalized.UI.RuntimeUpdateMode != config.UpdateCheckAutomatic {
		t.Fatalf("legacy settings = %q, %v", normalized.UI.RuntimeUpdateMode, err)
	}
	normalized.UI.RuntimeUpdateMode = "sometimes"
	if _, err := config.NormalizeSettings(normalized); err == nil {
		t.Fatal("an unknown runtime update mode was accepted")
	}
}
