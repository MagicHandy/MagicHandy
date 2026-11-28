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

func TestManualRuntimeInstallDrainsAndFencesLocalRequests(t *testing.T) {
	s := newTestServer(t)
	if !s.managedLLM.Snapshot().Runtime.BuildSupported {
		t.Skip("requires an installable runtime")
	}
	s.stopLLMAutoload()
	saveSettings(t, s.store, func(v config.Settings) config.Settings {
		v.LLM.ManagedLoadPolicy = config.LLMManagedLoadOnDemand
		return v
	})
	worker := &closeTrackingLLMProvider{}
	s.llm.cached = worker
	started := make(chan struct{}, 1)
	s.runtimeUpdater = func(string) (llm.ManagedLlamaRuntimeBuild, error) {
		started <- struct{}{}
		return llm.ManagedLlamaRuntimeBuild{Status: llm.RuntimeBuildStatusQueued}, nil
	}
	local, _, release, err := s.llmRequests.acquire(t.Context(), llmRequestInteractive)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err = s.installManagedRuntime(t.Context(), "invalid"); err == nil || local.Err() != nil || worker.closes.Load() != 0 {
		t.Fatal("invalid install disrupted the worker")
	}
	result := make(chan error, 1)
	go func() { _, err := s.installManagedRuntime(t.Context(), "cpu"); result <- err }()
	select {
	case <-local.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("install did not cancel active work")
	}
	select {
	case <-started:
		t.Error("installer started before local generation drained")
	default:
	}
	release()
	if err := <-result; err != nil || worker.closes.Load() != 1 {
		t.Fatal("installer failed to retire the worker", err)
	}
	if _, err := s.installManagedRuntime(t.Context(), "cpu"); err == nil {
		t.Error("duplicate install accepted")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if _, _, got, err := s.llmRequests.acquire(ctx, llmRequestInteractive); err == nil {
		got()
		t.Fatal("local generation admitted before installation finished")
	}
	ctx2, cancel2 := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel2()
	_, _, ready, err := s.llmRequests.acquire(ctx2, llmRequestInteractive)
	if err != nil {
		t.Fatal(err)
	}
	ready()
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
