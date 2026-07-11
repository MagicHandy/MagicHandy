package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectManagedLlamaRuntimeValidatesAppOwnedManifest(t *testing.T) {
	dataDir := t.TempDir()
	runnerRelative := "installs/b9966-cpu-c749cb0/bin/llama-server.exe"
	writeManagedRuntimeFixture(t, dataDir, managedRuntimeManifest{
		SchemaVersion: managedRuntimeManifestVersion,
		Runtime:       "llama.cpp",
		Version:       ManagedLlamaVersion,
		Commit:        ManagedLlamaCommit,
		Backend:       "cpu",
		Runner:        runnerRelative,
		Source:        "built_from_source",
		BuiltAt:       time.Now().UTC().Format(time.RFC3339Nano),
	})

	status := InspectManagedLlamaRuntime(dataDir)
	if !status.Installed || !status.Current || status.State != ManagedRuntimeStateReady {
		t.Fatalf("runtime status = %+v", status)
	}
	wantRunner := filepath.Join(ManagedLlamaRuntimeRoot(dataDir), filepath.FromSlash(runnerRelative))
	if status.RunnerPath != wantRunner || status.Version != ManagedLlamaVersion || status.Backend != "cpu" {
		t.Fatalf("runtime metadata = %+v, want runner %q", status, wantRunner)
	}
}

func TestInspectManagedLlamaRuntimeRejectsEscapingAndTrailingManifestData(t *testing.T) {
	for _, test := range []struct {
		name     string
		runner   string
		trailing bool
	}{
		{name: "escaping runner", runner: "../../llama-server.exe"},
		{name: "trailing data", runner: "installs/test/bin/llama-server.exe", trailing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			manifest := managedRuntimeManifest{
				SchemaVersion: managedRuntimeManifestVersion,
				Runtime:       "llama.cpp",
				Version:       ManagedLlamaVersion,
				Commit:        ManagedLlamaCommit,
				Backend:       "cpu",
				Runner:        test.runner,
				Source:        "built_from_source",
				BuiltAt:       time.Now().UTC().Format(time.RFC3339Nano),
			}
			writeManagedRuntimeFixture(t, dataDir, manifest)
			if test.trailing {
				path := filepath.Join(ManagedLlamaRuntimeRoot(dataDir), "active.json")
				file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- temp fixture path.
				if err != nil {
					t.Fatalf("open manifest: %v", err)
				}
				_, _ = file.WriteString("{}")
				_ = file.Close()
			}

			status := InspectManagedLlamaRuntime(dataDir)
			if status.Installed || status.State != ManagedRuntimeStateInvalid {
				t.Fatalf("runtime status = %+v, want invalid", status)
			}
		})
	}
}

func TestInspectManagedLlamaRuntimeReportsOutdatedAppBuild(t *testing.T) {
	dataDir := t.TempDir()
	writeManagedRuntimeFixture(t, dataDir, managedRuntimeManifest{
		SchemaVersion: managedRuntimeManifestVersion,
		Runtime:       "llama.cpp",
		Version:       "b9000",
		Commit:        "0123456789abcdef0123456789abcdef01234567",
		Backend:       "cuda",
		Runner:        "installs/old/bin/llama-server.exe",
		Source:        "built_from_source",
		BuiltAt:       time.Now().UTC().Format(time.RFC3339Nano),
	})
	status := InspectManagedLlamaRuntime(dataDir)
	if !status.Installed || status.Current || status.State != ManagedRuntimeStateOutdated {
		t.Fatalf("runtime status = %+v, want installed outdated build", status)
	}
}

func writeManagedRuntimeFixture(t *testing.T, dataDir string, manifest managedRuntimeManifest) {
	t.Helper()
	root := ManagedLlamaRuntimeRoot(dataDir)
	runner := filepath.Join(root, filepath.FromSlash(manifest.Runner))
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create runtime directory: %v", err)
	}
	if pathWithin(root, runner) {
		if err := os.MkdirAll(filepath.Dir(runner), 0o700); err != nil {
			t.Fatalf("create runner directory: %v", err)
		}
		if err := os.WriteFile(runner, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("write runner: %v", err)
		}
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "active.json"), payload, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
