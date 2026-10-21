package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/startup"
)

func TestDiagnosticReportIncludesRetainedStartupWithoutIdentity(t *testing.T) {
	server := newTestServer(t)
	dataDir := server.store.DataDir()
	instance, err := startup.Acquire(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	if err := instance.Publish("http://127.0.0.1:49717", "http://127.0.0.1:49718"); err != nil {
		t.Fatal(err)
	}
	identityJSON, err := os.ReadFile(filepath.Join(dataDir, "startup", "instance.json")) // #nosec G304 -- fixed file in the test-owned temporary profile.
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		ID    string `json:"instance_id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(identityJSON, &identity); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"failed-start", "recovered-start", "current-start"} {
		recorder := startup.NewRecorder(dataDir, version, "startup-commit")
		var failure error
		switch version {
		case "failed-start":
			failure = fmt.Errorf("PRIVATE_STARTUP_ERROR: %w", syscall.Errno(10013))
			recorder.Bind("app", "127.0.0.1:49717", failure)
		case "recovered-start":
			recorder.Bind("app", "127.0.0.1:49717", syscall.Errno(10048))
			recorder.Bind("app", "127.0.0.1:49719", nil)
		}
		if _, err := recorder.Finish(failure); err != nil {
			t.Fatal(err)
		}
	}
	// Startup is part of status even when every optional section is excluded.
	files := diagnosticFiles(t, server, "?settings=false&traces=false&chat=none")
	var status struct {
		Startup startup.Reports `json:"startup"`
	}
	if err := json.Unmarshal([]byte(files["status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	reports := status.Startup
	if reports.Current == nil || reports.LastFailure == nil || reports.LastRecovery == nil {
		t.Fatal("diagnostic status omitted retained startup records")
	}
	if reports.Current.AppVersion != "current-start" || reports.Current.Outcome != "ready" ||
		reports.LastFailure.AppVersion != "failed-start" || reports.LastFailure.NativeCode != 10013 ||
		reports.LastRecovery.AppVersion != "recovered-start" || reports.LastRecovery.Outcome != "recovered" {
		t.Fatalf("diagnostic status lost original startup evidence: %+v", reports)
	}
	assertDiagnosticExcludes(t, files, identity.Token, identity.ID, dataDir, "http://127.0.0.1:49717", "PRIVATE_STARTUP_ERROR")
}

func TestDiagnosticReportAllowsMissingStartupHistory(t *testing.T) {
	files := diagnosticFiles(t, newTestServer(t), "?settings=false&traces=false&chat=none")
	var status map[string]json.RawMessage
	if err := json.Unmarshal([]byte(files["status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	if string(status["startup"]) != `{"current":null,"last_failure":null,"last_recovery":null}` {
		t.Fatalf("missing startup history should be explicit: %s", status["startup"])
	}
}
