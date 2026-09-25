package console

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

// StopOutcome says what a Stop from the console achieved.
type StopOutcome int

const (
	// StopConfirmed means the device acknowledged the stop.
	StopConfirmed StopOutcome = iota
	// StopUnconfirmed means the stop was sent but the device did not confirm it.
	StopUnconfirmed
	// StopNothingConnected means no device was set up to receive a stop.
	StopNothingConnected
)

// Actions are what the console keys do. The dashboard only calls them.
type Actions struct {
	// Open opens the app in the default browser.
	Open func() error
	// Copy puts the app link on the clipboard. Nil hides the key.
	Copy func() error
	// Stop stops motion through the app's emergency-stop path.
	Stop func(context.Context) (StopOutcome, error)
	// Quit begins a clean shutdown of the app.
	Quit func()
}

const (
	frameInterval   = 350 * time.Millisecond
	quitConfirmTime = 4 * time.Second
	stopTimeout     = 20 * time.Second
	maxEntries      = 400
	eventBuffer     = 512
)

// Dashboard draws the launch console. Logging, keys and status changes are
// posted to one goroutine that owns all state and drawing, so a slow or
// paused console never holds up the app.
type Dashboard struct {
	term    Terminal
	now     func() time.Time
	events  chan func(*dashState)
	keys    chan rune
	quit    chan struct{}
	done    chan struct{}
	readEnd chan struct{}
	dropped atomic.Int64
	state   *dashState

	startOnce  sync.Once
	finishOnce sync.Once
	started    atomic.Bool
}

type dashState struct {
	view         view
	entries      []entry
	actions      Actions
	noticeUntil  time.Time
	quitArmed    time.Time
	quitting     bool
	title        string
	drawn        []string
	drawnColumns int
	drawnRows    int
}

// New returns a dashboard for the terminal. Call Start to show it.
func New(term Terminal, version string) *Dashboard {
	g := basicGlyphs
	if term.RichGlyphs() {
		g = richGlyphs
	}
	return &Dashboard{
		term:    term,
		now:     time.Now,
		events:  make(chan func(*dashState), eventBuffer),
		keys:    make(chan rune, 8),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
		readEnd: make(chan struct{}),
		state: &dashState{view: view{version: version, glyphs: g, clickable: term.RichGlyphs()},
			title: "MagicHandy"},
	}
}

// Start switches the terminal to the dashboard and begins drawing.
func (d *Dashboard) Start() {
	d.startOnce.Do(func() {
		d.started.Store(true)
		_, _ = d.term.Write([]byte("\x1b[?1049h\x1b[?25l" + frameStart + "\x1b[2J" + titleSequence(d.state.title)))
		go d.readKeys()
		go d.loop()
	})
}

// SetServer records where the app can be opened and how it can be reached.
func (d *Dashboard) SetServer(url, access string, simulated bool) {
	d.update(func(s *dashState) {
		s.view.url, s.view.access, s.view.simulated = url, access, simulated
		s.title = "MagicHandy" + " - " + url
		_, _ = d.term.Write([]byte(titleSequence(s.title)))
	})
}

// SetActions connects the keys to the app.
func (d *Dashboard) SetActions(actions Actions) {
	d.update(func(s *dashState) {
		s.actions = actions
		s.view.canCopy = actions.Copy != nil
	})
}

// SetRunning marks the app ready to open.
func (d *Dashboard) SetRunning() {
	d.update(func(s *dashState) {
		s.view.phase = phaseRunning
		s.view.since = d.now()
	})
}

// SetStopping marks the app as shutting down.
func (d *Dashboard) SetStopping() {
	d.update(func(s *dashState) { s.view.phase = phaseStopping })
}

// Finish leaves the dashboard and restores the terminal. A last line says
// the app stopped, or why it could not keep running. When this process alone
// owns the window, which closes as soon as it exits, it waits for a key so
// the reason can be read. It reports whether it showed err.
func (d *Dashboard) Finish(err error) bool {
	shown := false
	d.finishOnce.Do(func() {
		if d.started.Load() {
			close(d.quit)
			<-d.done
		}
		var out strings.Builder
		out.WriteString("\x1b[0m\x1b[?25h\x1b[?1049l")
		if err != nil {
			out.WriteString(colorWarn.foreground() + "MagicHandy stopped: " + safeText(err.Error()) + "\x1b[0m\r\n")
			shown = true
		} else {
			out.WriteString("MagicHandy stopped.\r\n")
		}
		wait := err != nil && d.term.SoleOwner()
		if wait {
			out.WriteString("Press any key to close this window.\r\n")
		}
		_, _ = d.term.Write([]byte(out.String()))
		if wait {
			if !d.started.Load() {
				go d.readKeys()
			}
			for len(d.keys) > 0 {
				<-d.keys // keys pressed earlier must not dismiss the reason
			}
			select {
			case <-d.keys:
			case <-d.readEnd:
			}
		}
		_ = d.term.Restore()
	})
	return shown
}

// update applies a status change. It waits for the drawing goroutine unless
// that has already finished.
func (d *Dashboard) update(fn func(*dashState)) {
	if !d.started.Load() {
		fn(d.state)
		return
	}
	select {
	case d.events <- fn:
	case <-d.done:
	}
}

// post adds activity without ever blocking the caller; if the console falls
// far behind, lines are counted and skipped.
func (d *Dashboard) post(fn func(*dashState)) {
	select {
	case d.events <- fn:
	default:
		d.dropped.Add(1)
	}
}

func (d *Dashboard) readKeys() {
	defer close(d.readEnd)
	for {
		key, err := d.term.ReadKey()
		if err != nil {
			return
		}
		select {
		case d.keys <- key:
		default:
		}
	}
}

func (d *Dashboard) loop() {
	defer close(d.done)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	s := d.state
	d.draw(s)
	for {
		select {
		case fn := <-d.events:
			fn(s)
			d.drainEvents(s)
		case key := <-d.keys:
			// Apply updates posted before the key first, such as the actions
			// the key calls.
			d.drainEvents(s)
			d.handleKey(s, key)
		case <-ticker.C:
			s.view.frame++
		case <-d.quit:
			return
		}
		d.draw(s)
	}
}

func (d *Dashboard) drainEvents(s *dashState) {
	for {
		select {
		case fn := <-d.events:
			fn(s)
		default:
			return
		}
	}
}

func (s *dashState) add(e entry) {
	s.entries = append(s.entries, e)
	if len(s.entries) > maxEntries {
		s.entries = append(s.entries[:0:0], s.entries[len(s.entries)-maxEntries:]...)
	}
}

func (d *Dashboard) note(level slog.Level, message, detail string) {
	e := entry{at: d.now(), level: level, message: message, detail: detail}
	d.update(func(s *dashState) { s.add(e) })
}

func (d *Dashboard) handleKey(s *dashState, key rune) {
	now := d.now()
	switch unicode.ToLower(key) {
	case 'o':
		if open := s.actions.Open; open != nil && s.view.url != "" {
			go func() {
				if err := open(); err != nil {
					d.note(slog.LevelWarn, "The browser did not open", err.Error())
					return
				}
				d.note(slog.LevelInfo, "Opened MagicHandy in your browser", "")
			}()
		}
	case 'c':
		if copyLink := s.actions.Copy; copyLink != nil && s.view.url != "" {
			go func() {
				if err := copyLink(); err != nil {
					d.note(slog.LevelWarn, "The link was not copied", err.Error())
					return
				}
				d.note(slog.LevelInfo, "Copied the link", "")
			}()
		}
	case 's', 0x1b:
		if stop := s.actions.Stop; stop != nil {
			s.add(entry{at: now, message: "Stopping motion from this window"})
			go d.stopMotion(stop)
		}
	case 'd':
		s.view.details = !s.view.details
	case 'q':
		d.quitKey(s, now)
	}
}

func (d *Dashboard) stopMotion(stop func(context.Context) (StopOutcome, error)) {
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	outcome, err := stop(ctx)
	switch {
	case err != nil:
		d.note(slog.LevelWarn, "Stop could not reach the device", err.Error())
	case outcome == StopConfirmed:
		d.note(slog.LevelInfo, "Motion stopped", "")
	case outcome == StopNothingConnected:
		d.note(slog.LevelInfo, "Nothing to stop: no device is connected", "")
	default:
		d.note(slog.LevelWarn, "Stop sent, but the device did not confirm it", "")
	}
}

// quitKey asks for a second Q, so a stray key press in the wrong window never
// ends a session.
func (d *Dashboard) quitKey(s *dashState, now time.Time) {
	if s.quitting || s.actions.Quit == nil {
		return
	}
	if now.Before(s.quitArmed) {
		s.quitting = true
		s.quitArmed = time.Time{}
		s.view.notice, s.view.noticeColor, s.noticeUntil = "Shutting down MagicHandy...", colorMuted, now.Add(time.Hour)
		go s.actions.Quit()
		return
	}
	s.quitArmed = now.Add(quitConfirmTime)
	s.view.notice, s.view.noticeColor = "Press Q again to quit MagicHandy. Motion stops and the app closes.", colorWarn
	s.noticeUntil = s.quitArmed
}

func (d *Dashboard) draw(s *dashState) {
	columns, rows := d.term.Size()
	if columns < 20 || rows < 8 {
		return
	}
	now := d.now()
	if s.view.notice != "" && now.After(s.noticeUntil) {
		s.view.notice = ""
	}
	v := s.view
	v.columns, v.rows = columns, rows
	for _, e := range s.entries {
		if v.details || !e.verbose {
			v.entries = append(v.entries, e)
		}
	}
	if skipped := d.dropped.Load(); skipped > 0 {
		v.entries = append(v.entries, entry{at: now, level: slog.LevelWarn,
			message: strconv.FormatInt(skipped, 10) + " log lines were skipped while the console caught up"})
	}
	frame := compose(v)
	var out strings.Builder
	if columns != s.drawnColumns || rows != s.drawnRows {
		out.WriteString(frameStart + "\x1b[2J")
		s.drawn = nil
	}
	drawn := make([]string, len(frame))
	for i, row := range frame {
		drawn[i] = row.render(columns - 1)
		if i < len(s.drawn) && s.drawn[i] == drawn[i] {
			continue
		}
		out.WriteString("\x1b[" + strconv.Itoa(i+1) + ";1H" + drawn[i] + frameStart + "\x1b[K")
	}
	if len(frame) < len(s.drawn) {
		out.WriteString("\x1b[" + strconv.Itoa(len(frame)+1) + ";1H" + frameStart + "\x1b[J")
	}
	s.drawn, s.drawnColumns, s.drawnRows = drawn, columns, rows
	if out.Len() > 0 {
		_, _ = d.term.Write([]byte(out.String()))
	}
}

func titleSequence(title string) string { return "\x1b]2;" + safeText(title) + "\x07" }
