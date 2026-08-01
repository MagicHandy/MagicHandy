//go:build windows

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestManagedLLMDuplicateAPIRequiresConfirmationAndRevalidatesProcess(t *testing.T) {
	runHTTPDuplicateHelper()

	server := newTestServer(t)
	runnerPath := installHTTPDuplicateRunnerFixture(t, server.store.DataDir())
	command, waited := startHTTPDuplicateRunnerFixture(t, runnerPath)
	terminated := false
	t.Cleanup(func() {
		if terminated {
			return
		}
		_ = command.Process.Kill()
		select {
		case <-waited:
		case <-time.After(3 * time.Second):
		}
	})

	snapshot := waitForHTTPDuplicateSnapshot(t, server, command.Process.Pid)
	requestBody := map[string]any{"pids": []int{command.Process.Pid}}
	assertHTTPDuplicateTerminationDenied(t, server, requestBody)

	confirmed := httptest.NewRecorder()
	server.Handler().ServeHTTP(confirmed, withController(jsonAPIRequest(t, http.MethodPost, "/api/llm/duplicates/terminate", requestBody)))
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirmed termination = %d: %s", confirmed.Code, confirmed.Body.String())
	}
	if err := json.Unmarshal(confirmed.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Processes) != 0 {
		t.Fatalf("remaining duplicate processes = %+v", snapshot.Processes)
	}
	select {
	case <-waited:
		terminated = true
	case <-time.After(3 * time.Second):
		t.Fatal("confirmed duplicate process did not terminate")
	}
}

func runHTTPDuplicateHelper() {
	if os.Getenv("MAGICHANDY_HTTP_DUPLICATE_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Second)
	}
}

func installHTTPDuplicateRunnerFixture(t *testing.T, dataDir string) string {
	t.Helper()
	writeHTTPAPIManagedRuntime(t, dataDir)
	runnerPath := llm.InspectManagedLlamaRuntime(dataDir).RunnerPath
	// #nosec G304,G703 -- this test copies its own fixed test executable into a
	// temporary app-owned managed-runtime fixture.
	payload, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- runnerPath is derived from the validated temporary
	// app-owned runtime manifest created by writeHTTPAPIManagedRuntime.
	if err := os.WriteFile(runnerPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return runnerPath
}

func startHTTPDuplicateRunnerFixture(t *testing.T, runnerPath string) (*exec.Cmd, <-chan error) {
	t.Helper()
	// #nosec G204,G702 -- this test intentionally executes the fixed fixture
	// copied above and controls its helper environment variable.
	command := exec.Command(runnerPath, "-test.run=TestManagedLLMDuplicateAPIRequiresConfirmationAndRevalidatesProcess")
	command.Env = append(os.Environ(), "MAGICHANDY_HTTP_DUPLICATE_HELPER=1")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	return command, waited
}

func waitForHTTPDuplicateSnapshot(t *testing.T, server *Server, pid int) managedLLMDuplicateSnapshot {
	t.Helper()
	var snapshot managedLLMDuplicateSnapshot
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/llm/duplicates", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("duplicate status = %d: %s", recorder.Code, recorder.Body.String())
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Processes) == 1 && snapshot.Processes[0].PID == pid {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !snapshot.Managed || len(snapshot.Processes) != 1 || snapshot.Processes[0].PID != pid {
		t.Fatalf("duplicate snapshot = %+v, want helper PID %d", snapshot, pid)
	}
	return snapshot
}

func assertHTTPDuplicateTerminationDenied(t *testing.T, server *Server, requestBody map[string]any) {
	t.Helper()
	denied := httptest.NewRecorder()
	server.Handler().ServeHTTP(denied, jsonAPIRequest(t, http.MethodPost, "/api/llm/duplicates/terminate", requestBody))
	if denied.Code != http.StatusConflict {
		t.Fatalf("uncontrolled termination = %d, want %d: %s", denied.Code, http.StatusConflict, denied.Body.String())
	}
}
