//go:build windows

package console

import (
	"os"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

type windowsTerminal struct {
	in, out         windows.Handle
	inMode, outMode uint32
	rich            bool

	mu       sync.Mutex
	restored bool
	pending  []rune
}

// Open prepares the process console for the dashboard. It returns
// ErrNotInteractive when input, output or errors are redirected, or when the
// console cannot process terminal sequences (Windows 10 before version 1511).
func Open() (Terminal, error) {
	in := windows.Handle(os.Stdin.Fd())
	out := windows.Handle(os.Stdout.Fd())
	errOut := windows.Handle(os.Stderr.Fd())
	var inMode, outMode, errMode uint32
	if windows.GetConsoleMode(in, &inMode) != nil || windows.GetConsoleMode(out, &outMode) != nil ||
		windows.GetConsoleMode(errOut, &errMode) != nil {
		return nil, ErrNotInteractive
	}
	if windows.SetConsoleMode(out, outMode|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
		return nil, ErrNotInteractive
	}
	// Keys arrive one at a time without echo. Processed input stays on, so
	// Ctrl+C still raises the interrupt that shuts the app down cleanly.
	keys := inMode&^(windows.ENABLE_LINE_INPUT|windows.ENABLE_ECHO_INPUT|windows.ENABLE_VIRTUAL_TERMINAL_INPUT) | windows.ENABLE_PROCESSED_INPUT
	if windows.SetConsoleMode(in, keys) != nil {
		_ = windows.SetConsoleMode(out, outMode)
		return nil, ErrNotInteractive
	}
	return &windowsTerminal{in: in, out: out, inMode: inMode, outMode: outMode, rich: hostedByTerminal()}, nil
}

func (t *windowsTerminal) Write(p []byte) (int, error) { return os.Stdout.Write(p) }

func (t *windowsTerminal) Size() (int, int) {
	var info windows.ConsoleScreenBufferInfo
	if windows.GetConsoleScreenBufferInfo(t.out, &info) != nil {
		return 80, 25
	}
	return int(info.Window.Right-info.Window.Left) + 1, int(info.Window.Bottom-info.Window.Top) + 1
}

// ReadKey reads the console directly: os.Stdin treats Ctrl+Z as end of input,
// which would silently end the keys.
func (t *windowsTerminal) ReadKey() (rune, error) {
	for len(t.pending) == 0 {
		var buffer [16]uint16
		var read uint32
		if err := windows.ReadConsole(t.in, &buffer[0], uint32(len(buffer)), &read, nil); err != nil {
			return 0, err
		}
		runes := utf16.Decode(buffer[:read])
		if len(runes) > 1 && runes[0] == 0x1b {
			continue // an escape sequence from a special key, not a lone Escape
		}
		t.pending = runes
	}
	key := t.pending[0]
	t.pending = t.pending[1:]
	return key, nil
}

func (t *windowsTerminal) Restore() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.restored {
		return nil
	}
	t.restored = true
	inErr := windows.SetConsoleMode(t.in, t.inMode)
	if err := windows.SetConsoleMode(t.out, t.outMode); err != nil {
		return err
	}
	return inErr
}

func (t *windowsTerminal) RichGlyphs() bool { return t.rich }

func (t *windowsTerminal) SoleOwner() bool {
	if procGetConsoleProcessList.Find() != nil {
		return false
	}
	var processes [2]uint32
	// #nosec G103 -- GetConsoleProcessList fills this fixed array; it is not retained.
	count, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&processes[0])), uintptr(len(processes)))
	return count == 1
}

// hostedByTerminal reports whether a ConPTY host such as Windows Terminal,
// rather than the classic console window, draws this console. Windows 11
// hands console apps to Windows Terminal by default without setting
// WT_SESSION, so the console's pseudo window class decides.
func hostedByTerminal() bool {
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	if procGetConsoleWindow.Find() != nil {
		return false
	}
	window, _, _ := procGetConsoleWindow.Call()
	if window == 0 {
		return false
	}
	var name [64]uint16
	copied, err := windows.GetClassName(windows.HWND(window), &name[0], int32(len(name)))
	if err != nil || copied <= 0 {
		return false
	}
	return windows.UTF16ToString(name[:copied]) == "PseudoConsoleWindow"
}
