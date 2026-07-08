package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestFreestyleProceduralDispatchesChaosByStyle(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		settings.Motion.Style = config.MotionStyleIntense
		return settings
	})

	started := personalizationRequest(t, server, http.MethodPost, "/api/modes/start", `{"mode":"freestyle"}`)
	if started.Code != http.StatusOK {
		t.Fatalf("start freestyle = %d: %s", started.Code, started.Body.String())
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		counts := countModeCommands(fake.Commands())
		if counts.plays > 0 && counts.adds > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("procedural freestyle missing HSP dispatch: %+v", fake.Commands())
}

func TestFreestyleLibraryModeStillUsesEngine(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeLibrary
		return settings
	})

	started := personalizationRequest(t, server, http.MethodPost, "/api/modes/start", `{"mode":"freestyle"}`)
	if started.Code != http.StatusOK {
		t.Fatalf("start freestyle = %d: %s", started.Code, started.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("library freestyle did not start motion engine")
}
