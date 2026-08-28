package accounts

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileImageRoundTripReplacementAndDelete(t *testing.T) {
	store, _ := newAccountStore(t)
	account, err := store.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	first := jpegFixture(t, 64, 80)
	updated, err := store.SaveProfileImage(t.Context(), account.ID, first)
	if err != nil {
		t.Fatalf("SaveProfileImage: %v", err)
	}
	if !updated.HasProfileImage || updated.ProfileUpdatedAt == "" {
		t.Fatalf("profile metadata = %+v", updated)
	}
	file, err := store.OpenProfileImage(t.Context(), account.ID)
	if err != nil {
		t.Fatalf("OpenProfileImage: %v", err)
	}
	stored, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || !bytes.Equal(stored, first) {
		t.Fatalf("stored profile differs: read=%v equal=%t", err, bytes.Equal(stored, first))
	}

	now = now.Add(time.Second)
	replaced, err := store.SaveProfileImage(t.Context(), account.ID, jpegFixture(t, 72, 72))
	if err != nil {
		t.Fatalf("replace profile: %v", err)
	}
	if replaced.ProfileUpdatedAt == updated.ProfileUpdatedAt {
		t.Fatal("profile replacement did not move cache timestamp")
	}

	cleared, err := store.DeleteProfileImage(t.Context(), account.ID)
	if err != nil {
		t.Fatalf("DeleteProfileImage: %v", err)
	}
	if cleared.HasProfileImage || cleared.ProfileUpdatedAt != "" {
		t.Fatalf("cleared profile metadata = %+v", cleared)
	}
	if _, err := store.OpenProfileImage(t.Context(), account.ID); !errors.Is(err, ErrProfileImageNotFound) {
		t.Fatalf("open after delete = %v, want ErrProfileImageNotFound", err)
	}
}

func TestProfileImageRejectsInvalidAndReconcilesOrphans(t *testing.T) {
	store, database := newAccountStore(t)
	account, err := store.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	for name, payload := range map[string][]byte{
		"not jpeg":      []byte("not an image"),
		"oversize edge": jpegFixture(t, MaxProfileImageEdge+1, 2),
	} {
		if _, err := store.SaveProfileImage(t.Context(), account.ID, payload); !errors.Is(err, ErrProfileImageInvalid) {
			t.Fatalf("%s error = %v, want ErrProfileImageInvalid", name, err)
		}
	}
	if err := os.MkdirAll(store.ProfileImageDir(), 0o700); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	orphan := filepath.Join(store.ProfileImageDir(), "ffffffffffffffffffffffffffffffff.jpg")
	if err := os.WriteFile(orphan, jpegFixture(t, 8, 8), 0o600); err != nil {
		t.Fatalf("write orphan: %v", err)
	}
	partial := filepath.Join(store.ProfileImageDir(), account.ID+".jpg.partial")
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	if _, err := New(database); err != nil {
		t.Fatalf("reconcile profiles: %v", err)
	}
	for _, path := range []string{orphan, partial} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("reconcile left %s: %v", path, err)
		}
	}
}

func jpegFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 96, A: 255})
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return output.Bytes()
}
