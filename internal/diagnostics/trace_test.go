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
		Kind:          transport.CommandKindPointsAdd,
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
			PositionPercent: 42.25,
			TimeMillis:      125,
		},
		TransportResult: &result,
	})

	got, err := json.Marshal(ring.Export())
	if err != nil {
		t.Fatalf("marshal trace export: %v", err)
	}
	want := `{"schema_version":"motion_trace.v3","rows":[{"sequence":1,"timestamp":"2026-06-30T12:00:00Z","source":"test","reason":"fixture","target":{"label":"manual","speed_percent":50,"stroke_min_percent":10,"stroke_max_percent":90},"sample":{"position_percent":42.25,"time_ms":125},"transport_result":{"command_id":"fake-000001","kind":"points_add","transport":"fake_handy","ok":true,"status":"recorded","latency_ms":7,"completed_at":"2026-06-30T12:00:00Z"}}],"dropped_rows":0,"intiface_dispatches_dropped":0,"intiface_linear_sent_count":0}`
	if string(got) != want {
		t.Fatalf("trace export mismatch\nwant: %s\ngot:  %s", want, got)
	}
}

func TestTraceRingCapacity(t *testing.T) {
	ring := NewTraceRing(2)
	for index, reason := range []string{"first", "second", "third", "fourth", "fifth"} {
		ring.Add(MotionTraceRow{Timestamp: "2026-06-30T12:00:00Z", Source: "test", Reason: reason})
		if summary := ring.Summary(); summary.RowCount > summary.Capacity {
			t.Fatalf("row count exceeded capacity after add %d: %+v", index, summary)
		}
	}

	export := ring.Export()
	if len(export.Rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(export.Rows))
	}
	if export.Rows[0].Reason != "fourth" || export.Rows[1].Reason != "fifth" {
		t.Fatalf("rows = %+v, want fourth and fifth", export.Rows)
	}
	if export.DroppedRows != 3 {
		t.Fatalf("dropped rows = %d, want 3", export.DroppedRows)
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

func TestTraceRingOwnsAddedRows(t *testing.T) {
	ring := NewTraceRing(2)
	target := &MotionTraceTarget{
		Label:        "original",
		SpeedPercent: 30,
	}
	planner := &MotionTracePlanner{
		Mode: "freestyle",
		Scores: []PlannerScore{
			{PatternIdentifier: "a", Score: 1, Chosen: true},
		},
	}
	retarget := &MotionTraceRetarget{
		PreviousTarget: &MotionTraceTarget{Label: "previous"},
		NextTarget:     &MotionTraceTarget{Label: "next"},
	}
	command := transport.Command{
		ID:   "fake-000001",
		Kind: transport.CommandKindPointsAdd,
		PointsAdd: &transport.AppendPointsCommand{
			StreamID: "1",
			Points: []transport.TimedPoint{
				{PositionPercent: 20, TimeMillis: 100},
			},
		},
	}

	added := ring.Add(MotionTraceRow{
		Timestamp:        "2026-06-30T12:00:00Z",
		Source:           "test",
		Reason:           "clone",
		Target:           target,
		Planner:          planner,
		Retarget:         retarget,
		TransportCommand: &command,
	})

	added.Target.Label = "mutated-return"
	added.Planner.Scores[0].Score = 100
	added.Retarget.PreviousTarget.Label = "mutated-return-previous"
	added.TransportCommand.PointsAdd.Points[0].PositionPercent = 100
	target.Label = "mutated"
	planner.Scores[0].Score = 99
	retarget.PreviousTarget.Label = "mutated-previous"
	command.PointsAdd.Points[0].PositionPercent = 99

	row := ring.Export().Rows[0]
	if row.Target.Label != "original" {
		t.Fatalf("target label = %q, want original", row.Target.Label)
	}
	if row.Planner.Scores[0].Score != 1 {
		t.Fatalf("planner score = %v, want 1", row.Planner.Scores[0].Score)
	}
	if row.Retarget.PreviousTarget.Label != "previous" {
		t.Fatalf("previous target label = %q, want previous", row.Retarget.PreviousTarget.Label)
	}
	if row.TransportCommand.PointsAdd.Points[0].PositionPercent != 20 {
		t.Fatalf("transport point = %g, want 20", row.TransportCommand.PointsAdd.Points[0].PositionPercent)
	}
}

func TestTraceRowsAndExportReturnIndependentSnapshots(t *testing.T) {
	ring := NewTraceRing(2)
	ring.Add(MotionTraceRow{
		Timestamp: "2026-06-30T12:00:00Z",
		Source:    "test",
		Reason:    "snapshot",
		Target: &MotionTraceTarget{
			Label:        "stored",
			SpeedPercent: 30,
		},
		Planner: &MotionTracePlanner{
			Mode: "freestyle",
			Scores: []PlannerScore{
				{PatternIdentifier: "a", Score: 1, Chosen: true},
			},
		},
		Retarget: &MotionTraceRetarget{
			PreviousTarget: &MotionTraceTarget{Label: "previous"},
		},
	})

	rows := ring.Rows()
	rows[0].Target.Label = "mutated-rows"
	rows[0].Planner.Scores[0].Score = 99

	export := ring.Export()
	export.Rows[0].Retarget.PreviousTarget.Label = "mutated-export"

	row := ring.Export().Rows[0]
	if row.Target.Label != "stored" {
		t.Fatalf("target label = %q, want stored", row.Target.Label)
	}
	if row.Planner.Scores[0].Score != 1 {
		t.Fatalf("planner score = %v, want 1", row.Planner.Scores[0].Score)
	}
	if row.Retarget.PreviousTarget.Label != "previous" {
		t.Fatalf("previous target label = %q, want previous", row.Retarget.PreviousTarget.Label)
	}
}
