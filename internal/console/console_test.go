package console

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

type fakeTerminal struct {
	mu       sync.Mutex
	out      strings.Builder
	keys     chan rune
	columns  int
	rows     int
	rich     bool
	sole     bool
	restored atomic.Bool
}

func newFakeTerminal(rich bool) *fakeTerminal {
	return &fakeTerminal{keys: make(chan rune, 8), columns: 100, rows: 30, rich: rich}
}

func (f *fakeTerminal) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.Write(p)
}

func (f *fakeTerminal) Size() (int, int) { return f.columns, f.rows }

func (f *fakeTerminal) ReadKey() (rune, error) {
	key, ok := <-f.keys
	if !ok {
		return 0, io.EOF
	}
	return key, nil
}

func (f *fakeTerminal) Restore() error   { f.restored.Store(true); return nil }
func (f *fakeTerminal) RichGlyphs() bool { return f.rich }
func (f *fakeTerminal) SoleOwner() bool  { return f.sole }

func (f *fakeTerminal) output() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.String()
}

var escapes = regexp.MustCompile("\x1b\\][^\x1b\x07]*(\x1b\\\\|\x07)|\x1b\\[[0-9;?]*[A-Za-z]")

// visible drops terminal sequences, leaving what a person reads.
func visible(text string) string { return escapes.ReplaceAllString(text, "") }

func runningView(g glyphs, columns, rows int) view {
	return view{version: "0.1.0-alpha.48", url: "http://127.0.0.1:49717", access: "Local only (this computer)",
		phase: phaseRunning, since: time.Date(2026, 9, 25, 11, 4, 0, 0, time.UTC), glyphs: g, clickable: g.star == richGlyphs.star,
		canCopy: true, columns: columns, rows: rows,
		entries: []entry{{at: time.Date(2026, 9, 25, 11, 4, 2, 0, time.UTC), message: "Server starting", detail: "url=http://127.0.0.1:49717"}}}
}

func frameText(v view) string {
	var out []string
	for _, row := range compose(v) {
		out = append(out, visible(row.render(v.columns-1)))
	}
	return strings.Join(out, "\n")
}

// The window opens on the brand: a magic wand with a star and sparkles beside
// the MAGICHANDY wordmark, then where to open the app and the keys.
func TestFrameShowsWandWordmarkLinkAndKeys(t *testing.T) {
	text := frameText(runningView(richGlyphs, 100, 30))
	for _, want := range []string{"★", "╱", "█▀▄▀█ ▄▀█ █▀▀ █ █▀▀ █ █ ▄▀█ █▄ █ █▀▄ █▄█", "Local-first control for The Handy",
		"Version 0.1.0-alpha.48", "● Running since 11:04", "Open    http://127.0.0.1:49717", "Access  Local only (this computer)",
		"Server starting", "[O] Open in browser", "[C] Copy link", "[S] Stop motion", "[D] Details", "[Q] Quit",
		"Ctrl+click the link or press O"} {
		if !strings.Contains(text, want) {
			t.Fatalf("frame is missing %q:\n%s", want, text)
		}
	}
}

// The address is an OSC 8 hyperlink, which Windows Terminal opens on
// Ctrl+click; the plain text stays readable everywhere else.
func TestAddressIsAClickableLink(t *testing.T) {
	v := runningView(richGlyphs, 100, 30)
	var rendered string
	for _, row := range compose(v) {
		if text := row.render(99); strings.Contains(visible(text), "Open    ") {
			rendered = text
		}
	}
	want := "\x1b]8;;http://127.0.0.1:49717\x1b\\"
	if !strings.Contains(rendered, want) || !strings.Contains(rendered, "\x1b]8;;\x1b\\") {
		t.Fatalf("address row is not a hyperlink: %q", rendered)
	}
}

// Sparkles twinkle from frame to frame while the star stays at the wand tip.
// The classic console gets ASCII sparkles its fonts can draw.
func TestWandSparklesTwinkle(t *testing.T) {
	seen := map[string]bool{}
	for frame := range 6 {
		rows := wandFrame(richGlyphs, frame)
		var text []string
		for _, row := range rows {
			text = append(text, visible(row.render(wandWidth)))
		}
		joined := strings.Join(text, "\n")
		if !strings.Contains(joined, "★") {
			t.Fatalf("frame %d lost the star:\n%s", frame, joined)
		}
		seen[joined] = true
	}
	if len(seen) < 3 {
		t.Fatalf("the sparkles do not change between frames: %d distinct frames", len(seen))
	}
	for frame := range 6 {
		for _, row := range wandFrame(basicGlyphs, frame) {
			for _, r := range visible(row.render(wandWidth)) {
				if r > 0x7e {
					t.Fatalf("basic wand uses %q, which classic console fonts may lack", r)
				}
			}
		}
	}
}

// No row is wider than the window, so the terminal never wraps the layout.
func TestRowsFitTheWindow(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 25}, {64, 20}, {50, 16}, {30, 10}} {
		for _, g := range []glyphs{richGlyphs, basicGlyphs} {
			v := runningView(g, size[0], size[1])
			v.entries = append(v.entries, entry{at: v.since, level: slog.LevelWarn, message: strings.Repeat("long warning ", 20), detail: strings.Repeat("x", 300)})
			rows := compose(v)
			if len(rows) != size[1] {
				t.Fatalf("%dx%d frame has %d rows", size[0], size[1], len(rows))
			}
			for _, row := range rows {
				if width := utf8.RuneCountInString(visible(row.render(size[0] - 1))); width > size[0]-1 {
					t.Fatalf("%dx%d row is %d columns: %q", size[0], size[1], width, visible(row.render(size[0]-1)))
				}
			}
		}
	}
}

func TestHandlerShowsActivityAndHidesRequestsUntilDetails(t *testing.T) {
	d := New(newFakeTerminal(true), "dev")
	logger := slog.New(d.Handler(slog.LevelInfo)).With("component", "server")
	logger.Info("server starting", "url", "http://127.0.0.1:49717")
	logger.Info("http request", "method", "GET", "path", "/api/state", "status", 200, "bytes", 10, "duration_ms", 3)
	logger.Warn("device \x1b[31mlost", "error", "timeout\x07")
	logger.Debug("not shown at info")
	d.drainEvents(d.state)

	if len(d.state.entries) != 3 {
		t.Fatalf("entries = %+v", d.state.entries)
	}
	started, request, warning := d.state.entries[0], d.state.entries[1], d.state.entries[2]
	if started.message != "Server starting" || started.detail != "component=server  url=http://127.0.0.1:49717" || started.verbose {
		t.Fatalf("lifecycle entry = %+v", started)
	}
	if !request.verbose || request.message != "GET /api/state 200 (3 ms)" {
		t.Fatalf("request entry = %+v", request)
	}
	if strings.ContainsAny(warning.message+warning.detail, "\x1b\x07") {
		t.Fatalf("control characters reached the console: %q %q", warning.message, warning.detail)
	}

	v := runningView(richGlyphs, 100, 30)
	v.entries = nil
	for _, e := range d.state.entries {
		if !e.verbose {
			v.entries = append(v.entries, e)
		}
	}
	if strings.Contains(frameText(v), "GET /api/state") {
		t.Fatal("request lines show without details")
	}
}

// Logging never waits on the console: when the dashboard falls behind, lines
// are counted and skipped.
func TestPostNeverBlocks(t *testing.T) {
	d := New(newFakeTerminal(true), "dev")
	d.started.Store(true)
	logger := slog.New(d.Handler(slog.LevelInfo))
	finished := make(chan struct{})
	go func() {
		for range eventBuffer + 50 {
			logger.Info("busy")
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("logging blocked on the console")
	}
	if d.dropped.Load() != 50 {
		t.Fatalf("dropped = %d, want 50", d.dropped.Load())
	}
}

func startedDashboard(t *testing.T, actions Actions) (*Dashboard, *fakeTerminal) {
	t.Helper()
	term := newFakeTerminal(true)
	d := New(term, "dev")
	d.Start()
	d.SetServer("http://127.0.0.1:49717", "Local only (this computer)", false)
	d.SetActions(actions)
	d.SetRunning()
	t.Cleanup(func() { d.Finish(nil) })
	return d, term
}

func TestKeysCallTheirActions(t *testing.T) {
	opened := make(chan struct{}, 1)
	stopped := make(chan struct{}, 2)
	_, term := startedDashboard(t, Actions{
		Open: func() error { opened <- struct{}{}; return nil },
		Stop: func(context.Context) (StopOutcome, error) { stopped <- struct{}{}; return StopConfirmed, nil },
		Quit: func() {},
	})
	term.keys <- 'o'
	term.keys <- 'S'
	term.keys <- 0x1b
	for _, ch := range []chan struct{}{opened, stopped, stopped} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("a key did not reach its action")
		}
	}
	waitFor(t, func() bool { return strings.Contains(visible(term.output()), "Motion stopped") })
}

// One Q only asks; a second Q within a few seconds quits. A stray key press
// in the wrong window never ends a session.
func TestQuitNeedsASecondPress(t *testing.T) {
	var quits atomic.Int32
	_, term := startedDashboard(t, Actions{Quit: func() { quits.Add(1) }})
	term.keys <- 'q'
	waitFor(t, func() bool { return strings.Contains(visible(term.output()), "Press Q again to quit MagicHandy") })
	if quits.Load() != 0 {
		t.Fatal("one Q quit the app")
	}
	term.keys <- 'Q'
	waitFor(t, func() bool { return quits.Load() == 1 })
}

// When the app cannot keep running and the window closes with it, the reason
// stays on screen until a key is pressed.
func TestFinishShowsTheErrorAndWaitsWhenTheWindowWouldClose(t *testing.T) {
	term := newFakeTerminal(false)
	term.sole = true
	d := New(term, "dev")
	d.Start()
	done := make(chan bool)
	go func() { done <- d.Finish(errors.New("listen tcp 127.0.0.1:49717: address already in use")) }()
	waitFor(t, func() bool { return strings.Contains(term.output(), "Press any key to close this window.") })
	select {
	case <-done:
		t.Fatal("Finish returned before a key was pressed")
	case <-time.After(50 * time.Millisecond):
	}
	term.keys <- 'x'
	if shown := <-done; !shown {
		t.Fatal("Finish did not report that it showed the error")
	}
	out := term.output()
	if !strings.Contains(out, "\x1b[?1049l") || !strings.Contains(out, "MagicHandy stopped: listen tcp 127.0.0.1:49717: address already in use") || !term.restored.Load() {
		t.Fatalf("terminal was not restored with the error shown: %q", out)
	}
}

func TestFinishWithoutErrorDoesNotWait(t *testing.T) {
	term := newFakeTerminal(true)
	term.sole = true
	d := New(term, "dev")
	d.Start()
	if d.Finish(nil) {
		t.Fatal("Finish reported an error it was not given")
	}
	if !strings.Contains(term.output(), "MagicHandy stopped.") || !term.restored.Load() {
		t.Fatalf("clean exit output = %q", term.output())
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
