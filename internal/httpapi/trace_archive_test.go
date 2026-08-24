package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestStoppedMotionPersistsRedactedEffectivePaceTrace(t *testing.T) {
	ring := diagnostics.NewTraceRing(256)
	server := newTestServerWithRuntime(t, Runtime{
		Traces: ring, Transport: transport.NewFake(), MotionTransport: transport.NewFake(),
	})
	engine, admission, err := server.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := server.store.Snapshot()
	definition := motion.NormalizeDynamicDefinition(motion.DynamicDefinition{
		CenterPercent: 50, SpanPercent: 38, SpanMinPercent: 20,
		SpanProfile: motion.DynamicSpanProfileWander, VariationPercent: 68,
	})
	if _, err := engine.StartAtGeneration(t.Context(), motion.MotionTarget{
		Source: "archive_test", Dynamic: &definition, SpeedPercent: 72,
	}, settings.Motion, admission); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Stop(t.Context(), "archive_test_stop"); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/traces/last-motion", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var archive motionTraceArchive
	if err := json.Unmarshal(recorder.Body.Bytes(), &archive); err != nil {
		t.Fatal(err)
	}
	if archive.SchemaVersion != lastMotionTraceArchiveSchema || len(archive.Trace.Rows) == 0 {
		t.Fatalf("archive = %+v", archive)
	}
	last := archive.Trace.Rows[len(archive.Trace.Rows)-1]
	if last.Reason != "archive_test_stop" || last.TransportCommand == nil ||
		last.TransportCommand.Kind != transport.CommandKindStop {
		t.Fatalf("final archived row = %+v", last)
	}
	foundPace := false
	for _, row := range archive.Trace.Rows {
		if row.Target != nil && row.Target.EffectiveSpeedPercent > 0 {
			foundPace = row.Target.PaceLimited && len(row.Target.PaceLimiters) > 0
		}
	}
	if !foundPace {
		t.Fatalf("archive omitted effective pace diagnostics: %+v", archive.Trace.Rows)
	}
}

func TestLastMotionTraceArchiveIsBoundedRedactedAndDurable(t *testing.T) {
	ring := diagnostics.NewTraceRing(256)
	server := newTestServerWithRuntime(t, Runtime{Traces: ring})
	dataDir := server.store.DataDir()
	for index := 0; index < 180; index++ {
		ring.Add(diagnostics.MotionTraceRow{
			Timestamp: time.Unix(int64(index), 0).UTC().Format(time.RFC3339Nano),
			Source:    "archive_test", Reason: "sample",
			Sample: &diagnostics.MotionTraceSample{PositionPercent: float64(index % 100), TimeMillis: int64(index * 125)},
		})
	}
	secret := "never-persist-this-connection-key"
	command := transport.Command{
		Kind: transport.CommandKindStop,
		Stop: &transport.StopCommand{Reason: secret},
	}
	ring.Add(diagnostics.MotionTraceRow{
		Source: "archive_test", Reason: "stop", TransportCommand: &command,
	})
	server.persistLastMotionTrace("ignored", 1)
	archive, ok, err := server.loadLastMotionTrace(context.Background())
	if err != nil || !ok {
		t.Fatalf("load archive: ok=%v err=%v", ok, err)
	}
	if len(archive.Trace.Rows) > lastMotionTraceMaximumRows || archive.RowsOmitted == 0 {
		t.Fatalf("archive bounds = rows %d omitted %d", len(archive.Trace.Rows), archive.RowsOmitted)
	}
	document, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(document) > lastMotionTraceMaximumBytes {
		t.Fatalf("archive size = %d", len(document))
	}
	if strings.Contains(string(document), secret) {
		t.Fatalf("archive leaked transport secret: %s", document)
	}

	server.Close()
	reopenedStore, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	reopened := newTestServerWithStore(t, reopenedStore, Runtime{})
	recorder := httptest.NewRecorder()
	reopened.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/traces/last-motion", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), lastMotionTraceArchiveSchema) {
		t.Fatalf("reopened status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestTracePersistenceFailureCannotFailStop(t *testing.T) {
	server := newTestServer(t)
	engine, admission, err := server.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := server.store.Snapshot()
	definition := motion.NormalizeDynamicDefinition(motion.DynamicDefinition{
		CenterPercent: 50, SpanPercent: 40, SpanMinPercent: 20,
	})
	if _, err := engine.StartAtGeneration(t.Context(), motion.MotionTarget{
		Source: "archive_failure_test", Dynamic: &definition, SpeedPercent: 40,
	}, settings.Motion, admission); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.Datastore().SQL().Exec(`DROP TABLE app_kv`); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Stop(t.Context(), "archive_failure_stop")
	if err != nil {
		t.Fatalf("Stop inherited diagnostics failure: %v", err)
	}
	if state.Running || state.Starting || state.Completing || state.Paused {
		t.Fatalf("Stop did not leave a terminal engine state: %+v", state)
	}
}
