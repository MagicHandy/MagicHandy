package console

import (
	"log/slog"
	"strconv"
	"strings"
	"testing"
)

func dashboardScreen(d *Dashboard) string {
	d.drainEvents(d.state)
	d.draw(d.state)
	return visible(strings.Join(d.state.drawn, "\n"))
}

func TestPollingDoesNotEraseVisibleActivity(t *testing.T) {
	d := New(newFakeTerminal(true), "dev")
	logger := slog.New(d.Handler(slog.LevelInfo))
	logger.Info("server starting")
	logger.Warn("connection needs attention")
	for i := range maxEntries * 3 {
		logger.Info(requestMessage, "method", "GET", "path", "/api/state", "status", 200, "duration_ms", i)
		// Exercise retention, not overflow of the asynchronous delivery queue.
		if i%64 == 0 {
			d.drainEvents(d.state)
		}
	}
	for _, details := range []bool{false, true, false} {
		d.state.view.details = details
		screen := dashboardScreen(d)
		if details {
			if !strings.Contains(screen, "GET /api/state 200") {
				t.Fatal("Details lost the recent request history")
			}
			continue
		}
		for _, want := range []string{"Server starting", "Connection needs attention"} {
			if !strings.Contains(screen, want) {
				t.Fatalf("background polling erased %q from the current frame:\n%s", want, screen)
			}
		}
		if strings.Contains(screen, "GET /api/state") {
			t.Fatal("ordinary polling leaked into the activity view")
		}
	}
}

func TestFailedRequestsRemainVisibleWithoutDetails(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		level  slog.Level
		want   slog.Level
	}{
		{"client error", 403, slog.LevelInfo, slog.LevelWarn},
		{"server error", 503, slog.LevelInfo, slog.LevelError},
		{"explicit warning", 200, slog.LevelWarn, slog.LevelWarn},
		{"explicit error", 200, slog.LevelError, slog.LevelError},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := New(newFakeTerminal(true), "dev")
			logger := slog.New(d.Handler(slog.LevelInfo))
			logger.Log(t.Context(), test.level, requestMessage, "method", "POST", "path", "/api/settings", "status", test.status)
			if screen := dashboardScreen(d); !strings.Contains(screen, "POST /api/settings") {
				t.Fatalf("request failure is invisible without Details:\n%s", screen)
			}
			if level := d.state.activity.snapshot()[0].level; level != test.want {
				t.Fatalf("request severity = %s, want %s", level, test.want)
			}
		})
	}
}

func TestHistoryRolloverDoesNotAllocate(t *testing.T) {
	s := &dashState{}
	for range maxEntries {
		s.add(entry{message: "activity"})
	}
	if allocations := testing.AllocsPerRun(1000, func() { s.add(entry{message: "activity"}) }); allocations != 0 {
		t.Fatalf("history rollover allocated %g times per entry", allocations)
	}
}

func TestEmptyActivityHasAnExplanation(t *testing.T) {
	d := New(newFakeTerminal(true), "dev")
	if screen := dashboardScreen(d); !strings.Contains(screen, "No activity to display yet.") {
		t.Fatalf("empty activity area has no explanation:\n%s", screen)
	}
}

func TestActivityHistoriesStayBoundedAndOrderedAfterWrap(t *testing.T) {
	s := &dashState{}
	for i := range maxEntries * 3 {
		s.add(entry{message: strconv.Itoa(i)})
		s.add(entry{message: "poll " + strconv.Itoa(i), verbose: true})
	}
	for _, history := range []*entryHistory{&s.entries, &s.activity} {
		if len(history.items) != maxEntries || cap(history.items) != maxEntries {
			t.Fatalf("history is not bounded: len=%d cap=%d", len(history.items), cap(history.items))
		}
	}
	for i, e := range s.activity.snapshot() {
		if e.verbose || e.message != strconv.Itoa(maxEntries*2+i) {
			t.Fatalf("activity[%d] is out of order: %+v", i, e)
		}
	}
	for i, e := range s.entries.snapshot() {
		want := strconv.Itoa(maxEntries*3 - maxEntries/2 + i/2)
		if i%2 != 0 {
			want = "poll " + want
		}
		if e.message != want {
			t.Fatalf("details[%d] = %q, want %q", i, e.message, want)
		}
	}
}
