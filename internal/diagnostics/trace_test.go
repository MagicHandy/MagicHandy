package diagnostics

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestTraceRowSerialization(t *testing.T) {
	ring := NewTraceRing(4)
	result := transport.CommandResult{
		CommandID:     "fake-000001",
		Kind:          transport.CommandKindHSPAdd,
		Transport:     "fake_handy",
		OK:            true,
		Status:        "recorded",
		LatencyMillis: 7,
		CompletedAt:   "2026-06-30T12:00:00Z",
	}
	ring.Add(MotionTraceRow{
		Timestamp: "2026-06-30T12:00:00Z",
		Source:    "test",
		Reason:    "fixture",
		Target: &MotionTraceTarget{
			Label:            "manual",
			SpeedPercent:     50,
			StrokeMinPercent: 10,
			StrokeMaxPercent: 90,
		},
		Sample: &MotionTraceSample{
			PositionPercent: 42,
			TimeMillis:      125,
		},
		TransportResult: &result,
	})

	got, err := json.Marshal(ring.Export())
	if err != nil {
		t.Fatalf("marshal trace export: %v", err)
	}
	want := `{"schema_version":"motion_trace.v1","rows":[{"sequence":1,"timestamp":"2026-06-30T12:00:00Z","source":"test","reason":"fixture","target":{"label":"manual","speed_percent":50,"stroke_min_percent":10,"stroke_max_percent":90},"sample":{"position_percent":42,"time_ms":125},"transport_result":{"command_id":"fake-000001","kind":"hsp_add","transport":"fake_handy","ok":true,"status":"recorded","latency_ms":7,"completed_at":"2026-06-30T12:00:00Z"}}],"dropped_rows":0}`
	if string(got) != want {
		t.Fatalf("trace export mismatch\nwant: %s\ngot:  %s", want, got)
	}
}

func TestTraceRingCapacity(t *testing.T) {
	ring := NewTraceRing(2)
	ring.Add(MotionTraceRow{Timestamp: "2026-06-30T12:00:00Z", Source: "test", Reason: "first"})
	ring.Add(MotionTraceRow{Timestamp: "2026-06-30T12:00:01Z", Source: "test", Reason: "second"})
	ring.Add(MotionTraceRow{Timestamp: "2026-06-30T12:00:02Z", Source: "test", Reason: "third"})

	export := ring.Export()
	if len(export.Rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(export.Rows))
	}
	if export.Rows[0].Reason != "second" || export.Rows[1].Reason != "third" {
		t.Fatalf("rows = %+v, want second and third", export.Rows)
	}
	if export.DroppedRows != 1 {
		t.Fatalf("dropped rows = %d, want 1", export.DroppedRows)
	}
}

func TestTraceExportRedactsTransportSecrets(t *testing.T) {
	ring := NewTraceRing(2)
	command := transport.Command{
		ID:   "fake-000001",
		Kind: transport.CommandKindStop,
		Stop: &transport.StopCommand{Reason: "secret-connection-key"},
	}
	result := transport.CommandResult{
		CommandID: "fake-000001",
		Kind:      transport.CommandKindStop,
		Transport: "fake_handy",
		Status:    "failed",
		Error:     "secret-connection-key",
	}
	ring.Add(MotionTraceRow{
		Timestamp:        "2026-06-30T12:00:00Z",
		Source:           "test",
		Reason:           "redaction",
		TransportCommand: &command,
		TransportResult:  &result,
	})

	data, err := json.Marshal(ring.Export())
	if err != nil {
		t.Fatalf("marshal trace export: %v", err)
	}
	if strings.Contains(string(data), "secret-connection-key") {
		t.Fatalf("trace export leaked secret: %s", data)
	}
}
