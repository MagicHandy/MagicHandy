package llm

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedRuntimeInstallerMaterializesPinnedLicense(t *testing.T) {
	manager, err := OpenManagedLlamaRuntimeManager(t.TempDir())
	if err != nil {
		t.Fatalf("open runtime manager: %v", err)
	}
	defer manager.Close()

	scriptPath, err := manager.installBuildScript()
	if err != nil {
		t.Fatalf("materialize installer assets: %v", err)
	}
	if filepath.Base(scriptPath) != "build-managed-llama.ps1" {
		t.Fatalf("installer path = %q", scriptPath)
	}
	wantLicense, err := managedRuntimeAssets.ReadFile("runtimeassets/LICENSE-llama.cpp")
	if err != nil {
		t.Fatalf("read embedded license: %v", err)
	}
	gotLicense, err := os.ReadFile(filepath.Join(filepath.Dir(scriptPath), "LICENSE-llama.cpp"))
	if err != nil {
		t.Fatalf("read materialized license: %v", err)
	}
	if !bytes.Equal(gotLicense, wantLicense) {
		t.Fatal("materialized llama.cpp license differs from embedded asset")
	}
}

func TestInspectManagedLlamaRuntimeValidatesAppOwnedManifest(t *testing.T) {
	dataDir := t.TempDir()
	runnerRelative := "installs/b11149-cpu-d2e5458/bin/llama-server.exe"
	writeManagedRuntimeFixture(t, dataDir, managedRuntimeManifest{
		SchemaVersion: managedRuntimeManifestVersion,
		Runtime:       "llama.cpp",
		Version:       ManagedLlamaVersion,
		Commit:        ManagedLlamaCommit,
		Backend:       "cpu",
		Runner:        runnerRelative,
		Source:        "verified_upstream_release",
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

func TestInspectManagedLlamaRuntimeAcceptsLegacySourceBuild(t *testing.T) {
	dataDir := t.TempDir()
	writeManagedRuntimeFixture(t, dataDir, managedRuntimeManifest{
		SchemaVersion: managedRuntimeManifestVersion,
		Runtime:       "llama.cpp",
		Version:       ManagedLlamaVersion,
		Commit:        ManagedLlamaCommit,
		Backend:       "cpu",
		Runner:        "installs/legacy/bin/llama-server.exe",
		Source:        "built_from_source",
		BuiltAt:       time.Now().UTC().Format(time.RFC3339Nano),
	})

	status := InspectManagedLlamaRuntime(dataDir)
	if !status.Installed || !status.Current || status.Source != "built_from_source" {
		t.Fatalf("legacy source-built runtime status = %+v", status)
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

func TestPruneSupersededRuntimeInstallsKeepsOnlyTheActiveRuntime(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"b9966-cuda-c749cb0", "b11149-cuda-d2e5458", "b11149-cuda-d2e5458.partial-abc"} {
		if err := os.MkdirAll(filepath.Join(root, "installs", name, "bin"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	active := filepath.Join(root, "installs", "b11149-cuda-d2e5458", "bin", "llama-server.exe")
	pruneSupersededRuntimeInstalls(root, active)
	entries, err := os.ReadDir(filepath.Join(root, "installs"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(names) != 2 || names[0] != "b11149-cuda-d2e5458" || names[1] != "b11149-cuda-d2e5458.partial-abc" {
		t.Fatalf("installs after pruning = %v, want only the active runtime and an in-progress stage", names)
	}
	// A runner outside the install tree prunes nothing.
	pruneSupersededRuntimeInstalls(root, filepath.Join(t.TempDir(), "llama-server.exe"))
	if entries, _ := os.ReadDir(filepath.Join(root, "installs")); len(entries) != 2 {
		t.Fatalf("pruning ran for a runner outside the install tree: %d entries", len(entries))
	}
}
