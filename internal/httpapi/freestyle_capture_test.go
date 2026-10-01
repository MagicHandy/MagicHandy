package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// TestFreestyleSessionCapture records real-time Freestyle sessions through the
// shared engine and a fake transport for visual review
// (docs/freestyle-stream-review-2026-10-01.md). It only runs when
// MAGICHANDY_FREESTYLE_CAPTURE names an ignored output file, and it never
// creates a device transport. MAGICHANDY_FREESTYLE_SECONDS sets the session
// length (default 150) and MAGICHANDY_FREESTYLE_STYLES a comma-free list of
// g/b/i letters for the Gentle, Balanced and Intense feels (default all
// three). MAGICHANDY_FREESTYLE_SHAPE and MAGICHANDY_FREESTYLE_SHAPE_MINUTES
// choose a session shape. MAGICHANDY_FREESTYLE_CHANGE_AT, in seconds, switches
// every session to the Intense feel focused at the tip at that moment, to
// record a live control change.
func TestFreestyleSessionCapture(t *testing.T) {
	path := os.Getenv("MAGICHANDY_FREESTYLE_CAPTURE")
	if path == "" {
		t.Skip("set MAGICHANDY_FREESTYLE_CAPTURE to an ignored output path to record Freestyle sessions")
	}
	seconds := 150
	if value, err := strconv.Atoi(os.Getenv("MAGICHANDY_FREESTYLE_SECONDS")); err == nil && value > 0 {
		seconds = value
	}
	styles := map[byte]string{'g': config.FreestyleFeelGentle, 'b': config.FreestyleFeelBalanced, 'i': config.FreestyleFeelIntense}
	shape := os.Getenv("MAGICHANDY_FREESTYLE_SHAPE")
	shapeMinutes, _ := strconv.Atoi(os.Getenv("MAGICHANDY_FREESTYLE_SHAPE_MINUTES"))
	changeAt, _ := strconv.Atoi(os.Getenv("MAGICHANDY_FREESTYLE_CHANGE_AT"))
	selection := os.Getenv("MAGICHANDY_FREESTYLE_STYLES")
	if selection == "" {
		selection = "gbi"
	}

	sessions := make([]*freestyleCaptureSession, 0, len(selection))
	for index := range len(selection) {
		style, ok := styles[selection[index]]
		if !ok {
			t.Fatalf("unknown style letter %q", selection[index])
		}
		fake := transport.NewFake()
		traces := diagnostics.NewTraceRing(16384)
		server := newTestServerWithRuntime(t, Runtime{Traces: traces, Transport: fake, MotionTransport: fake})
		saveSettings(t, server.store, func(settings config.Settings) config.Settings {
			settings.Freestyle.Feel = style
			if shape != "" {
				settings.Freestyle.Shape = shape
			}
			if shapeMinutes > 0 {
				settings.Freestyle.ShapeMinutes = shapeMinutes
			}
			return settings
		})
		manager, err := modes.NewManager(modes.Options{
			Ensure: func(context.Context) (modes.Engine, error) {
				engine, admission, err := server.motionEngineForStart()
				if err != nil {
					return nil, err
				}
				return admittedMotionEngine{Engine: engine, admission: admission}, nil
			},
			Current: func() modes.Engine {
				if engine := server.currentMotionEngine(); engine != nil {
					return engine
				}
				return nil
			},
			Settings:          func() config.MotionSettings { settings, _ := server.store.Snapshot(); return settings.Motion },
			FreestyleSettings: func() config.FreestyleSettings { settings, _ := server.store.Snapshot(); return settings.Freestyle },
			Traces:            traces,
			Seed:              int64(1000 + index),
		})
		if err != nil {
			t.Fatal(err)
		}
		server.modes = manager
		t.Cleanup(manager.Shutdown)
		settings, _ := server.store.Snapshot()
		sessions = append(sessions, &freestyleCaptureSession{fake: fake, traces: traces, manager: manager, style: style, limits: settings.Motion, server: server})
	}

	var wait sync.WaitGroup
	for _, current := range sessions {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := current.manager.Start(context.Background(), modes.ModeFreestyle); err != nil {
				t.Errorf("start %s: %v", current.style, err)
				return
			}
			if changeAt > 0 && changeAt < seconds {
				time.Sleep(time.Duration(changeAt) * time.Second)
				// Save directly: t.Fatal must not run on this goroutine.
				settings, _ := current.server.store.Snapshot()
				settings.Freestyle.Feel, settings.Freestyle.PacePercent, settings.Freestyle.FocusPercent = config.FreestyleFeelCustom, 85, 80
				if _, err := current.server.store.Save(settings); err != nil {
					t.Errorf("change %s: %v", current.style, err)
				}
				time.Sleep(time.Duration(seconds-changeAt) * time.Second)
			} else {
				time.Sleep(time.Duration(seconds) * time.Second)
			}
			if _, err := current.server.emergencyStop(context.Background(), "capture_complete"); err != nil {
				t.Errorf("stop %s: %v", current.style, err)
			}
		}()
	}
	wait.Wait()

	writeFreestyleCapture(t, path, seconds, sessions)
}

type freestyleCaptureSession struct {
	fake    *transport.Fake
	traces  *diagnostics.TraceRing
	manager *modes.Manager
	style   string
	limits  config.MotionSettings
	server  *Server
}

func writeFreestyleCapture(t *testing.T, path string, seconds int, sessions []*freestyleCaptureSession) {
	t.Helper()
	report := make([]map[string]any, 0, len(sessions))
	for _, current := range sessions {
		report = append(report, map[string]any{"style": current.style, "limits": current.limits, "seconds": seconds,
			"commands": current.fake.Commands(), "trace_rows": current.traces.Rows()})
	}
	data, err := json.MarshalIndent(map[string]any{"scenario": "Real-time Freestyle sessions on the shared engine and a fake transport", "sessions": report}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- Explicit opt-in local test artifact path, never an HTTP request or runtime setting.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
