package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/store"
)

func TestStoreAddToggleRemoveClearPersists(t *testing.T) {
	dir := store.TestDir(t)
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	first, err := store.Add("Likes slow starts.")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	second, err := store.Add("Prefers the tease pattern.")
	if err != nil {
		t.Fatalf("Add second: %v", err)
	}
	if !first.Enabled || first.ID == "" || first.CreatedAt == "" {
		t.Fatalf("first memory = %+v, want enabled with id and timestamp", first)
	}

	if _, err := store.SetItemEnabled(second.ID, false); err != nil {
		t.Fatalf("SetItemEnabled: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	snapshot := reopened.Snapshot()
	if len(snapshot.Memories) != 2 {
		t.Fatalf("persisted memories = %d, want 2", len(snapshot.Memories))
	}
	if snapshot.Memories[1].Enabled {
		t.Fatal("disabled memory did not persist as disabled")
	}

	if err := reopened.Remove(first.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := reopened.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	final, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen after clear: %v", err)
	}
	if got := len(final.Snapshot().Memories); got != 0 {
		t.Fatalf("memories after clear = %d, want 0", got)
	}
}

func TestPromptTextsHonorsItemAndGlobalSwitches(t *testing.T) {
	store, err := Open(store.TestDir(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	kept, err := store.Add("Kept memory.")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	muted, err := store.Add("Muted memory.")
	if err != nil {
		t.Fatalf("Add muted: %v", err)
	}
	if _, err := store.SetItemEnabled(muted.ID, false); err != nil {
		t.Fatalf("SetItemEnabled: %v", err)
	}

	texts := store.PromptTexts()
	if len(texts) != 1 || texts[0] != kept.Text {
		t.Fatalf("PromptTexts = %v, want only the enabled memory", texts)
	}

	if err := store.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if texts := store.PromptTexts(); texts != nil {
		t.Fatalf("PromptTexts with global switch off = %v, want nil", texts)
	}
	// The memories themselves survive the global switch.
	if got := len(store.Snapshot().Memories); got != 2 {
		t.Fatalf("memories after disable = %d, want 2", got)
	}
}

func TestStoreImportsLegacyMemoryFile(t *testing.T) {
	dir := t.TempDir()
	file := memoriesFile{
		Version: memoriesVersion,
		Enabled: false,
		Memories: []Memory{
			{ID: "mem-legacy-1", Text: "Legacy one.", Enabled: true, CreatedAt: "2026-07-05T00:00:00Z"},
			{ID: "mem-legacy-2", Text: "Legacy two.", Enabled: false, CreatedAt: "2026-07-05T00:00:01Z"},
		},
	}
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal legacy memory file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, memoriesFileName), data, 0o600); err != nil {
		t.Fatalf("write legacy memory file: %v", err)
	}

	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	snapshot := store.Snapshot()
	if snapshot.Enabled {
		t.Fatal("legacy global memory switch did not import as disabled")
	}
	if len(snapshot.Memories) != 2 {
		t.Fatalf("imported memories = %+v, want 2", snapshot.Memories)
	}
	if texts := store.PromptTexts(); texts != nil {
		t.Fatalf("PromptTexts with imported global switch off = %v, want nil", texts)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, memoriesFileName)); !os.IsNotExist(err) {
		t.Fatalf("legacy memory path stat = %v, want renamed away", err)
	}
	if _, err := os.Stat(filepath.Join(dir, memoriesFileName+".migrated")); err != nil {
		t.Fatalf("archived legacy memory file missing: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() {
		_ = reopened.Close()
	}()
	if got := reopened.Snapshot(); got.Enabled || len(got.Memories) != 2 {
		t.Fatalf("reopened imported snapshot = %+v, want disabled with 2 memories", got)
	}
}

func TestStoreValidatesInputAndUnknownIDs(t *testing.T) {
	store, err := Open(store.TestDir(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.Add("   "); err == nil {
		t.Fatal("Add accepted blank text")
	}
	if _, err := store.Add(strings.Repeat("x", maxMemoryChars+1)); err == nil {
		t.Fatal("Add accepted oversized text")
	}
	if _, err := store.SetItemEnabled("mem-missing", true); err != ErrMemoryNotFound {
		t.Fatalf("SetItemEnabled unknown id error = %v, want ErrMemoryNotFound", err)
	}
	if err := store.Remove("mem-missing"); err != ErrMemoryNotFound {
		t.Fatalf("Remove unknown id error = %v, want ErrMemoryNotFound", err)
	}
}

func TestStoreRecoversFromCorruptFileWithoutFailingStartup(t *testing.T) {
	dir := store.TestDir(t)
	if err := os.WriteFile(filepath.Join(dir, memoriesFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open corrupt: %v", err)
	}
	if !store.Recovered() {
		t.Fatal("corrupt file did not report recovery")
	}
	snapshot := store.Snapshot()
	if !snapshot.Enabled || len(snapshot.Memories) != 0 {
		t.Fatalf("recovered snapshot = %+v, want enabled and empty", snapshot)
	}
	// The store stays writable after recovery.
	if _, err := store.Add("Post-recovery memory."); err != nil {
		t.Fatalf("Add after recovery: %v", err)
	}
}

func TestStoreNormalizesLoadedMemoryFile(t *testing.T) {
	dir := store.TestDir(t)
	file := struct {
		Version  int      `json:"version"`
		Memories []Memory `json:"memories"`
	}{
		Version: memoriesVersion,
		Memories: []Memory{
			{ID: " mem-1 ", Text: "  Kept.  ", Enabled: true, CreatedAt: "2026-07-05T00:00:00Z"},
			{ID: "mem-1", Text: "Duplicate.", Enabled: true},
			{ID: "mem-blank", Text: " ", Enabled: true},
			{ID: "mem-big", Text: strings.Repeat("x", maxMemoryChars+1), Enabled: true},
		},
	}
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal memory file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, memoriesFileName), data, 0o600); err != nil {
		t.Fatalf("write memory file: %v", err)
	}

	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !store.Recovered() {
		t.Fatal("invalid loaded memory records should report recovery")
	}
	snapshot := store.Snapshot()
	if !snapshot.Enabled {
		t.Fatal("missing enabled switch should default to true")
	}
	if len(snapshot.Memories) != 1 {
		t.Fatalf("memories = %+v, want only one valid unique memory", snapshot.Memories)
	}
	if snapshot.Memories[0].ID != "mem-1" || snapshot.Memories[0].Text != "Kept." {
		t.Fatalf("memory = %+v, want trimmed valid record", snapshot.Memories[0])
	}
}
