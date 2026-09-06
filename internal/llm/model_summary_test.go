package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModelInventoryPreservesCancellationCause(t *testing.T) {
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = manager.List(ctx)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrModelInventoryUnavailable) {
		t.Fatalf("inventory lost cancellation classification: %v", err)
	}
}

func summaryFixture(t testing.TB, count int) (*ModelManager, []ModelRecord, []byte) {
	t.Helper()
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	data := testGGUFData(t, ggufFixtureOptions{})
	models := make([]ModelRecord, count)
	for i := range models {
		id := fmt.Sprintf("summary-%03d", i)
		directory := filepath.Join(manager.modelsDir, id)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		models[i] = ModelRecord{ID: id, DisplayName: id, Provider: "llama_cpp", Source: ModelSourceGGUF,
			Format: "gguf", SizeBytes: int64(len(data)), SHA256: fmt.Sprintf("%064x", i+1), ModelPath: filepath.Join(directory, "model.gguf")}
		if err := os.WriteFile(models[i].ModelPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := manager.insertModel(context.Background(), models[i]); err != nil {
			t.Fatal(err)
		}
	}
	return manager, models, data
}

func TestModelSummaryInspectsOnlySelectedFileAndReflectsChanges(t *testing.T) {
	manager, models, _ := summaryFixture(t, 3)
	manager.jobs = map[string]*modelImportJob{
		"queued":   {snapshot: ImportJob{Status: ImportStatusQueued}},
		"copying":  {snapshot: ImportJob{Status: ImportStatusCopying}},
		"complete": {snapshot: ImportJob{Status: ImportStatusComplete}},
	}
	summary, err := manager.Summary(t.Context(), "")
	if err != nil || summary.ModelCount != 3 || summary.ActiveImportCount != 2 || summary.SelectedReady {
		t.Fatalf("counts = %+v, %v", summary, err)
	}
	if len(manager.compatibility) != 0 {
		t.Fatal("counts inspected model files")
	}
	selected := models[0]
	summary, err = manager.Summary(t.Context(), selected.ID)
	if err != nil || !summary.SelectedReady || len(manager.compatibility) != 1 {
		t.Fatalf("selected summary = %+v, %v; inspected %d files", summary, err, len(manager.compatibility))
	}
}

func TestSelectedModelSummaryInvalidatesChangedAndRemovedFiles(t *testing.T) {
	manager, models, data := summaryFixture(t, 3)
	selected := models[0]
	if _, err := manager.Summary(t.Context(), selected.ID); err != nil {
		t.Fatal(err)
	}
	// Same-size replacement invalidates compatibility through its changed mtime.
	invalid := append([]byte(nil), data...)
	invalid[0] = 'X'
	if err := os.WriteFile(selected.ModelPath, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(time.Second)
	if err := os.Chtimes(selected.ModelPath, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if summary, err := manager.Summary(t.Context(), selected.ID); err != nil || summary.SelectedReady {
		t.Fatalf("replacement stayed ready: %+v, %v", summary, err)
	}
	if err := os.WriteFile(selected.ModelPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	stamp = stamp.Add(time.Second)
	if err := os.Chtimes(selected.ModelPath, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if summary, err := manager.Summary(t.Context(), selected.ID); err != nil || !summary.SelectedReady {
		t.Fatalf("restored file stayed unavailable: %+v, %v", summary, err)
	}
	if err := os.Remove(selected.ModelPath); err != nil {
		t.Fatal(err)
	}
	if summary, err := manager.Summary(t.Context(), selected.ID); err != nil || summary.SelectedReady || summary.ModelCount != 3 {
		t.Fatalf("removed file = %+v, %v", summary, err)
	}
	if err := manager.Delete(t.Context(), models[1].ID, selected.ID); err != nil {
		t.Fatal(err)
	}
	if summary, err := manager.Summary(t.Context(), "unknown"); err != nil || summary.ModelCount != 2 || summary.SelectedReady {
		t.Fatalf("changed inventory = %+v, %v", summary, err)
	}
}

func BenchmarkModelStatusInventory(b *testing.B) {
	manager, models, _ := summaryFixture(b, 128)
	for _, method := range []string{"full_snapshot", "selected_summary", "counts_only"} {
		b.Run(method, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				switch method {
				case "full_snapshot":
					_, err = manager.Snapshot(b.Context())
				case "selected_summary":
					_, err = manager.Summary(b.Context(), models[0].ID)
				case "counts_only":
					_, err = manager.Summary(b.Context(), "")
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
