package startup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReportsRetainFailureAndRecoveryWithoutRawPrivateData(t *testing.T) {
	root := t.TempDir()
	const secret = "private-key-and-C:\\Users\\private\\settings"
	failed := NewRecorder(root, "test", "commit")
	failed.Report.Stage = "bind_app"
	denial := fmt.Errorf("%s: %w", secret, syscall.Errno(10013))
	if !failed.Bind("app", "127.0.0.1:49717", denial) {
		t.Fatal("Windows bind denial should permit local fallback")
	}
	if _, err := failed.Finish(denial); err != nil {
		t.Fatal(err)
	}
	recovered := NewRecorder(root, "test", "commit")
	recovered.Bind("app", "127.0.0.1:49717", denial)
	recovered.Bind("app", "127.0.0.1:48800", nil)
	if _, err := recovered.Finish(nil); err != nil {
		t.Fatal(err)
	}
	clean := NewRecorder(root, "new-version", "new-commit")
	clean.Bind("app", "127.0.0.1:48800", nil)
	if _, err := clean.Finish(nil); err != nil {
		t.Fatal(err)
	}
	reports := ReadReports(root)
	if reports.Current.Outcome != "ready" || reports.LastFailure.Outcome != "failed" || reports.LastRecovery.Outcome != "recovered" || reports.LastFailure.NativeCode != 10013 || reports.LastFailure.AppVersion != "test" {
		t.Fatalf("reports=%+v", reports)
	}
	data, err := json.Marshal(reports)
	if err != nil || strings.Contains(string(data), secret) || strings.Contains(string(data), "127.0.0.1") {
		t.Fatalf("private data in reports: %s (%v)", data, err)
	}
}

func TestReportFallsBackWhenProfileIsUnwritable(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := NewRecorder(blocked, "test", "commit")
	path, err := recorder.Finish(errors.New("cannot open settings"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
	data, err := os.ReadFile(path) // #nosec G304 -- recorder-created private temporary report path.
	if err != nil || !strings.Contains(string(data), `"outcome": "failed"`) {
		t.Fatalf("fallback report=%s err=%v", data, err)
	}
}

func TestStartupDocumentsAreBoundedAndPortHintsMatchConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := SavePortHint(root, PortHint{Preference: "original", Main: 48800, Remote: 48801}); err != nil {
		t.Fatal(err)
	}
	if hint := ReadPortHint(root, "changed"); hint.Main != 0 {
		t.Fatal("changed preference reused a stale port")
	}
	if hint := ReadPortHint(root, "original"); hint.Main != 48800 {
		t.Fatal("working local origin was not remembered")
	}
	if err := os.WriteFile(filepath.Join(root, "startup", "current.json"), []byte(strings.Repeat("x", maxDocumentBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if ReadReports(root).Current != nil {
		t.Fatal("unbounded report was accepted")
	}
}
