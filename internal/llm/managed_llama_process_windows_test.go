//go:build windows

package llm

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedLlamaProcessJobClosesChild(t *testing.T) {
	if os.Getenv("MAGICHANDY_MANAGED_PROCESS_HELPER") == "1" {
		for {
			time.Sleep(time.Second)
		}
	}

	// #nosec G204,G702 -- this test intentionally re-executes its own fixed binary.
	command := exec.Command(os.Args[0], "-test.run=TestManagedLlamaProcessJobClosesChild")
	command.Env = append(os.Environ(), "MAGICHANDY_MANAGED_PROCESS_HELPER=1")
	configureManagedLlamaProcess(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	lifetime, err := attachManagedLlamaProcess(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	if err := lifetime.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("closing the managed process job did not terminate its child")
	}
}

func TestManagedRunnerProcessDetectionAndGuardedTermination(t *testing.T) {
	if os.Getenv("MAGICHANDY_DUPLICATE_PROCESS_HELPER") == "1" {
		for {
			time.Sleep(time.Second)
		}
	}

	// #nosec G204,G702 -- this test intentionally re-executes its own fixed binary.
	command := exec.Command(os.Args[0], "-test.run=TestManagedRunnerProcessDetectionAndGuardedTermination")
	command.Env = append(os.Environ(), "MAGICHANDY_DUPLICATE_PROCESS_HELPER=1")
	configureManagedLlamaProcess(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	terminated := false
	defer func() {
		if terminated {
			return
		}
		_ = command.Process.Kill()
		select {
		case <-waited:
		case <-time.After(3 * time.Second):
		}
	}()

	deadline := time.Now().Add(3 * time.Second)
	var found []ManagedRunnerProcess
	for time.Now().Before(deadline) {
		var err error
		found, err = FindManagedRunnerProcesses(os.Args[0], 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) == 1 && found[0].PID == command.Process.Pid {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(found) != 1 || found[0].PID != command.Process.Pid {
		t.Fatalf("managed runner processes = %+v, want child PID %d", found, command.Process.Pid)
	}
	if found[0].Executable != filepath.Base(os.Args[0]) {
		t.Fatalf("executable = %q, want %q", found[0].Executable, filepath.Base(os.Args[0]))
	}

	wrongPath := filepath.Join(t.TempDir(), "llama-server.exe")
	if err := TerminateManagedRunnerProcess(wrongPath, command.Process.Pid); err == nil {
		t.Fatal("termination with a mismatched executable path succeeded")
	}
	if err := TerminateManagedRunnerProcess(os.Args[0], command.Process.Pid); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waited:
		terminated = true
	case <-time.After(3 * time.Second):
		t.Fatal("guarded termination did not stop the matching process")
	}
}
