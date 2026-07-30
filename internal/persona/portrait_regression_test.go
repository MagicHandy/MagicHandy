package persona

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestPortraitRejectsAValidHeaderWithTruncatedScanData(t *testing.T) {
	store := newTestStore(t)
	item := createPersona(t, store, "Rowan")
	complete := jpegOfSize(t, 96, 96)

	var truncated []byte
	for cut := len(complete) - 1; cut > 3; cut-- {
		candidate := complete[:cut]
		if _, err := jpeg.DecodeConfig(bytes.NewReader(candidate)); err != nil {
			continue
		}
		if _, err := jpeg.Decode(bytes.NewReader(candidate)); err != nil {
			truncated = candidate
			break
		}
	}
	if truncated == nil {
		t.Fatal("fixture did not produce a JPEG whose header decodes but scan data does not")
	}
	if _, err := store.SavePortrait(context.Background(), item.ID, truncated); !errors.Is(err, ErrPortraitInvalid) {
		t.Fatalf("SavePortrait error = %v, want ErrPortraitInvalid", err)
	}
}

func TestPortraitRemovalFailureLeavesThePersonaRetryable(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		remove func(*Store, string) error
	}{
		{
			name: "clear portrait",
			remove: func(store *Store, id string) error {
				_, err := store.DeletePortrait(context.Background(), id)
				return err
			},
		},
		{
			name: "delete persona",
			remove: func(store *Store, id string) error {
				return store.Delete(context.Background(), id)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := newTestStore(t)
			item := createPersona(t, store, "Rowan")
			if _, err := store.SavePortrait(context.Background(), item.ID, jpegOfSize(t, 32, 40)); err != nil {
				t.Fatalf("save portrait: %v", err)
			}
			path, err := store.portraitPath(item.ID)
			if err != nil {
				t.Fatalf("portrait path: %v", err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatalf("replace portrait fixture: %v", err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatalf("create blocking directory: %v", err)
			}
			if err := os.WriteFile(filepath.Join(path, "locked"), []byte("fixture"), 0o600); err != nil {
				t.Fatalf("populate blocking directory: %v", err)
			}

			if err := testCase.remove(store, item.ID); err == nil {
				t.Fatal("portrait removal unexpectedly succeeded")
			}
			after, err := store.Get(context.Background(), item.ID)
			if err != nil {
				t.Fatalf("persona was no longer retryable: %v", err)
			}
			if !after.HasPortrait {
				t.Fatal("failed removal cleared the portrait record")
			}
		})
	}
}

func TestPortraitReplacementRestoresPreviousFileWhenStampFails(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")
	original := jpegOfSize(t, 32, 40)
	replacement := jpegOfSize(t, 48, 60)
	if _, err := store.SavePortrait(ctx, item.ID, original); err != nil {
		t.Fatalf("save original portrait: %v", err)
	}
	if _, err := store.db.SQL().Exec(`
		CREATE TRIGGER fail_portrait_stamp
		BEFORE UPDATE OF portrait_updated_at ON personas
		BEGIN
			SELECT RAISE(ABORT, 'fixture stamp failure');
		END
	`); err != nil {
		t.Fatalf("install stamp failure: %v", err)
	}

	if _, err := store.SavePortrait(ctx, item.ID, replacement); err == nil {
		t.Fatal("replacement unexpectedly succeeded")
	}
	stored, err := store.readPortrait(item.ID)
	if err != nil {
		t.Fatalf("read portrait after rollback: %v", err)
	}
	if !bytes.Equal(stored, original) {
		t.Fatal("failed database stamp left replacement bytes on disk")
	}
}

func TestStartupReconcilesInterruptedAndMissingPortraitFiles(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatalf("open persona store: %v", err)
	}
	item := createPersona(t, store, "Rowan")
	if _, err := store.SavePortrait(context.Background(), item.ID, jpegOfSize(t, 32, 40)); err != nil {
		t.Fatalf("save portrait: %v", err)
	}
	portraitPath, err := store.portraitPath(item.ID)
	if err != nil {
		t.Fatalf("portrait path: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}
	if err := os.Remove(portraitPath); err != nil {
		t.Fatalf("remove expected portrait: %v", err)
	}
	orphanPath := filepath.Join(filepath.Dir(portraitPath), "persona-ffffffffffff.jpg")
	if err := os.WriteFile(orphanPath, jpegOfSize(t, 16, 16), 0o600); err != nil {
		t.Fatalf("write orphaned portrait: %v", err)
	}
	partialPath := portraitPath + ".partial"
	if err := os.WriteFile(partialPath, []byte("interrupted"), 0o600); err != nil {
		t.Fatalf("write interrupted portrait: %v", err)
	}

	reopened, err := Open(dataDir)
	if err != nil {
		t.Fatalf("reopen persona store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	after, err := reopened.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("get reconciled persona: %v", err)
	}
	if after.HasPortrait {
		t.Fatal("missing portrait file left a stale database flag")
	}
	for _, path := range []string{orphanPath, partialPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("startup left %s: %v", filepath.Base(path), err)
		}
	}
}
