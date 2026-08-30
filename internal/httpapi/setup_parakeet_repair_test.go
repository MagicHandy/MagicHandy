package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParakeetDownloaderResumesAndReportsByteProgress(t *testing.T) {
	content := bytes.Repeat([]byte("verified-parakeet-asset"), 64*1024)
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	resumeAt := len(content) / 3
	rangeSeen := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeSeen = r.Header.Get("Range")
		if rangeSeen != fmt.Sprintf("bytes=%d-", resumeAt) {
			t.Errorf("Range = %q", rangeSeen)
			http.Error(w, "unexpected range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", resumeAt, len(content)-1, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[resumeAt:])
	}))
	defer server.Close()

	root := t.TempDir()
	asset := parakeetDownloadAsset{
		label:       "Parakeet test asset",
		url:         server.URL,
		destination: filepath.Join(root, "asset.bin"),
		sha256:      digest,
		maxBytes:    int64(len(content) + 1024),
	}
	if err := os.WriteFile(parakeetPartialPath(asset), content[:resumeAt], 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &setupManager{job: &setupJobState{setupJob: setupJob{ID: "download-test"}}}
	if err := manager.downloadVerifiedParakeetAsset(context.Background(), "download-test", asset); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(asset.destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, content) {
		t.Fatal("resumed download content differs")
	}
	if rangeSeen == "" {
		t.Fatal("download did not use a byte range")
	}
	job := manager.Snapshot()
	if job == nil || job.BytesCompleted != int64(len(content)) || job.BytesTotal != int64(len(content)) {
		t.Fatalf("download progress = %+v", job)
	}
}

func TestParakeetPreflightRejectsAnUnwritableTargetShape(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data-file")
	if err := os.WriteFile(dataDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := preflightParakeetInstall(dataDir); err == nil || !strings.Contains(err.Error(), "prepare Parakeet module directory") {
		t.Fatalf("preflight error = %v", err)
	}
}

func TestSetupResultPersistsBoundedSanitizedFailure(t *testing.T) {
	dataDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := newSetupManager(context.Background(), dataDir, "", logger, nil, nil)
	_, job, err := manager.reserveJob("parakeet", "parakeet", "cpu", "queued")
	if err != nil {
		t.Fatal(err)
	}
	unsafeMessage := "download failed\n\x00" + strings.Repeat("x", 700)
	manager.updateJob(job.ID, setupJobFailed, unsafeMessage, "runtime output is intentionally not persisted")
	manager.Close()

	restored := newSetupManager(context.Background(), dataDir, "", logger, nil, nil)
	defer restored.Close()
	snapshot := restored.Snapshot()
	if snapshot == nil || snapshot.Status != setupJobFailed {
		t.Fatalf("restored job = %+v", snapshot)
	}
	if strings.ContainsAny(snapshot.Message, "\n\x00") || len([]rune(snapshot.Message)) > setupResultTextLimit {
		t.Fatalf("restored message was not sanitized and bounded: %q", snapshot.Message)
	}
	if snapshot.Output != "" {
		t.Fatalf("runtime output persisted: %q", snapshot.Output)
	}
	info, err := os.Stat(filepath.Join(dataDir, setupResultFileName))
	if err != nil || info.Size() > setupResultFileLimit {
		t.Fatalf("persisted result size = %v, err=%v", info, err)
	}
}

func TestParakeetRepairStopsAndRestoresOnlyAfterVerification(t *testing.T) {
	dataDir := t.TempDir()
	manager := newSetupManager(context.Background(), dataDir, "", slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	defer manager.Close()
	manager.preflightParakeet = func(string) error { return nil }
	order := make([]string, 0, 5)
	manager.prepareParakeet = func(context.Context) (bool, error) {
		order = append(order, "stop")
		return true, nil
	}
	manager.downloadParakeet = func(context.Context, string) error {
		order = append(order, "download")
		return nil
	}
	manager.runParakeetInstaller = func(_ context.Context, _ string, result setupParakeetInstallResult) error {
		order = append(order, "install")
		if err := os.MkdirAll(filepath.Dir(result.ServerPath), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(result.ServerPath, []byte("runner"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(result.ModelPath, []byte("model"), 0o600)
	}
	manager.onParakeet = func(context.Context, setupParakeetInstallResult) error {
		order = append(order, "apply")
		return nil
	}
	manager.restoreParakeet = func(context.Context) error {
		order = append(order, "restore")
		return nil
	}
	manager.job = &setupJobState{setupJob: setupJob{ID: "repair-test"}}
	if err := manager.installParakeet(context.Background(), "repair-test"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"stop", "download", "install", "apply", "restore"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("repair order = %v, want %v", order, want)
	}
}

func TestParakeetRepairDoesNotRestoreAfterFailedVerification(t *testing.T) {
	manager := newSetupManager(context.Background(), t.TempDir(), "", slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	defer manager.Close()
	manager.preflightParakeet = func(string) error { return nil }
	manager.prepareParakeet = func(context.Context) (bool, error) { return true, nil }
	manager.downloadParakeet = func(context.Context, string) error { return nil }
	manager.runParakeetInstaller = func(context.Context, string, setupParakeetInstallResult) error { return nil }
	restored := false
	manager.restoreParakeet = func(context.Context) error { restored = true; return nil }
	manager.job = &setupJobState{setupJob: setupJob{ID: "failed-repair-test"}}
	if err := manager.installParakeet(context.Background(), "failed-repair-test"); err == nil {
		t.Fatal("repair without verified files succeeded")
	}
	if restored {
		t.Fatal("failed repair restored the previously running worker")
	}
}
