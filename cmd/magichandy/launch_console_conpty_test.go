//go:build windows && consoleharness

package main

// TestLaunchConsoleInPseudoConsole runs the real app in a hidden Windows
// pseudo console, as Windows Terminal hosts it, presses keys and checks what
// it draws. It builds the app and takes about ten seconds, so it is opt-in:
//
//	go test -tags consoleharness ./cmd/magichandy -run TestLaunchConsoleInPseudoConsole
//
// MAGICHANDY_CONSOLE_CAPTURE, when set, saves the raw terminal output there.

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type pseudoConsoleRun struct {
	mu       sync.Mutex
	captured strings.Builder
	input    windows.Handle
}

func (r *pseudoConsoleRun) output() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.captured.String()
}

func (r *pseudoConsoleRun) press(keys string) {
	var written uint32
	_ = windows.WriteFile(r.input, []byte(keys), &written, nil)
}

// terminalSequence matches OSC and CSI sequences, leaving the text.
var terminalSequence = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-9;?]*[ -/]*[@-~]`)

// waitFor waits until the raw terminal output contains the sequence.
func (r *pseudoConsoleRun) waitFor(t *testing.T, sequence string) {
	t.Helper()
	r.waitUntil(t, sequence, r.output)
}

// waitForText waits until the text, without styling or cursor movement,
// contains text.
func (r *pseudoConsoleRun) waitForText(t *testing.T, text string) {
	t.Helper()
	r.waitUntil(t, text, func() string { return terminalSequence.ReplaceAllString(r.output(), "") })
}

func (r *pseudoConsoleRun) waitUntil(t *testing.T, text string, read func() string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(read(), text) {
		if time.Now().After(deadline) {
			t.Fatalf("the console never showed %q", text)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func startInPseudoConsole(t *testing.T, commandLine string) (*pseudoConsoleRun, windows.Handle, func()) {
	t.Helper()
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		t.Fatal(err)
	}
	var console windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: 110, Y: 32}, inRead, outWrite, 0, &console); err != nil {
		t.Fatal(err)
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatal(err)
	}
	// The attribute takes the pseudo console handle itself as its value.
	value := *(*unsafe.Pointer)(unsafe.Pointer(&console)) // #nosec G103 -- reinterprets the handle's bits for the Win32 attribute.
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, value, unsafe.Sizeof(console)); err != nil {
		t.Fatal(err)
	}
	var startup windows.StartupInfoEx
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.ProcThreadAttributeList = attributes.List()
	// Null standard handles make the child use the pseudo console instead of
	// inheriting the test's redirected handles.
	startup.Flags = windows.STARTF_USESTDHANDLES
	line, err := windows.UTF16PtrFromString(commandLine)
	if err != nil {
		t.Fatal(err)
	}
	var process windows.ProcessInformation
	if err := windows.CreateProcess(nil, line, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &process); err != nil {
		t.Fatal(err)
	}
	run := &pseudoConsoleRun{input: inWrite}
	go func() {
		buffer := make([]byte, 64<<10)
		for {
			var read uint32
			if err := windows.ReadFile(outRead, buffer, &read, nil); err != nil || read == 0 {
				return
			}
			run.mu.Lock()
			run.captured.Write(buffer[:read])
			run.mu.Unlock()
		}
	}()
	cleanup := func() {
		_ = windows.TerminateProcess(process.Process, 1)
		windows.ClosePseudoConsole(console)
		attributes.Delete()
	}
	return run, process.Process, cleanup
}

func TestLaunchConsoleInPseudoConsole(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "magichandy.exe")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=0.1.0-alpha.99", "-o", executable, ".") // #nosec G204 -- builds this package into the test's temporary directory.
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	address := "127.0.0.1:" + strconv.Itoa(freeLoopbackPort(t))
	commandLine := `"` + executable + `" -simulate-motion -addr ` + address + ` -data-dir "` + filepath.Join(t.TempDir(), "data") + `"`
	run, process, cleanup := startInPseudoConsole(t, commandLine)
	defer cleanup()
	defer func() {
		if path := os.Getenv("MAGICHANDY_CONSOLE_CAPTURE"); path != "" {
			_ = os.WriteFile(path, []byte(run.output()), 0o600) // #nosec G703 -- explicit local capture path.
		}
	}()

	link := "http://" + address
	run.waitForText(t, "Running")
	// ConPTY passes the OSC 8 hyperlink on and adds an id parameter.
	for _, want := range []string{"\x1b[?1049h", "\x1b]8;", ";" + link + "\x1b\\", "MagicHandy - " + link} {
		run.waitFor(t, want)
	}
	for _, want := range []string{"Version 0.1.0-alpha.99", "Local-first control for The Handy", "Simulated; no device moves",
		"[O] Open in browser", "[S] Stop motion", "★"} {
		run.waitForText(t, want)
	}
	run.press("s")
	run.waitForText(t, "Motion stopped")
	run.press("q")
	run.waitForText(t, "Press Q again to quit MagicHandy")
	run.press("q")
	if event, err := windows.WaitForSingleObject(process, 15000); err != nil || event != windows.WAIT_OBJECT_0 {
		t.Fatalf("the app did not quit after Q, Q: %d %v", event, err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(process, &code); err != nil || code != 0 {
		t.Fatalf("exit code %d, %v", code, err)
	}
	run.waitForText(t, "MagicHandy stopped.")
}
