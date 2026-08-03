//go:build windows

package openaittsworker

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestManagedServerStopTerminatesOwnedProcessTree(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	childPort := freeLoopbackPort(t)
	server := newManagedServer(
		executable,
		[]string{"-test.run=^TestManagedServerTreeHelperProcess$"},
		"",
		freeLoopbackPort(t),
		map[string]string{
			"MAGICHANDY_TTS_TREE_HELPER": "parent",
			"MAGICHANDY_TTS_CHILD_PORT":  strconv.Itoa(childPort),
		},
	)
	t.Cleanup(func() { _ = server.Stop() })
	if err := server.Start(); err != nil {
		t.Fatalf("start managed server tree helper: %v", err)
	}
	if err := waitForPortState(childPort, false, 15*time.Second); err != nil {
		t.Fatalf("managed server child did not start: %v; output: %q", err, server.output.String())
	}

	if err := server.Stop(); err != nil {
		t.Fatalf("stop managed server tree helper: %v", err)
	}
	if err := waitForPortState(childPort, true, 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestManagedServerTreeHelperProcess(_ *testing.T) {
	if os.Getenv("MAGICHANDY_TTS_TREE_CHILD") == "1" {
		port, err := strconv.Atoi(os.Getenv("MAGICHANDY_TTS_CHILD_PORT"))
		if err != nil {
			os.Exit(2)
		}
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			os.Exit(2)
		}
		defer func() { _ = listener.Close() }()
		for {
			time.Sleep(time.Hour)
		}
	}
	if os.Getenv("MAGICHANDY_TTS_TREE_HELPER") == "parent" {
		executable, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		// #nosec G204,G702 -- the test restarts its own resolved executable with a fixed argument.
		command := exec.Command(executable, "-test.run=^TestManagedServerTreeHelperProcess$")
		command.Env = append(os.Environ(), "MAGICHANDY_TTS_TREE_CHILD=1")
		if err := command.Start(); err != nil {
			os.Exit(2)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
}

func waitForPortState(port int, available bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		isAvailable := err == nil
		if listener != nil {
			_ = listener.Close()
		}
		if isAvailable == available {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("port %d available = %t, want %t (last error: %v)", port, isAvailable, available, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
