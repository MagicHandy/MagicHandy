package motion

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func countCommands(commands []transport.Command, kind transport.CommandKind) int {
	count := 0
	for _, command := range commands {
		if command.Kind == kind {
			count++
		}
	}
	return count
}

func assertRefreshedStrokeWindow(t *testing.T, commands []transport.Command) {
	t.Helper()
	for _, command := range commands {
		if command.StrokeWindow == nil {
			continue
		}
		if command.StrokeWindow.MinPercent == 10 && command.StrokeWindow.MaxPercent == 90 {
			return
		}
	}
	t.Fatalf("commands = %+v, want refreshed 10..90 stroke window", commands)
}

func assertNoRestartBeforeStop(t *testing.T, commands []transport.Command) {
	t.Helper()

	playCount := 0
	stopCount := 0
	for _, command := range commands {
		switch command.Kind {
		case transport.CommandKindPointsPlay:
			playCount++
		case transport.CommandKindStop:
			stopCount++
		}
	}
	if playCount != 1 || stopCount != 0 {
		t.Fatalf("commands = %+v, want one play and no regular stop before explicit stop", commands)
	}
}

func assertNoTraceRestartBeforeStop(t *testing.T, rows []diagnostics.MotionTraceRow) {
	t.Helper()

	playCount := 0
	stopCount := 0
	for _, row := range rows {
		switch row.Reason {
		case "play":
			playCount++
		case "stop":
			stopCount++
		}
	}
	if playCount != 1 || stopCount != 0 {
		t.Fatalf("trace rows = %+v, want one play and no regular stop before explicit stop", rows)
	}
}

// assertEngineEmitsSemanticPosition proves the engine sends the semantic
// 0..100 position even when reverse is enabled: reverse is a transport-boundary
// mapping (Invariant 3), so the engine must not pre-invert (which would
// double-invert on the real Cloud/Bluetooth transports). The reverse mapping
// itself is covered by the transport tests.
func assertEngineEmitsSemanticPosition(t *testing.T, commands []transport.Command, sample *MotionSample) {
	t.Helper()
	if sample == nil {
		t.Fatal("last semantic sample is missing")
	}
	add := lastPointsAdd(commands)
	if add == nil || len(add.Points) == 0 {
		t.Fatalf("commands = %+v, want HSP add points", commands)
	}
	got := add.Points[len(add.Points)-1].PositionPercent
	if got != sample.PositionPercent {
		t.Fatalf("last transport point = %g, want semantic sample %g (engine must not pre-reverse)", got, sample.PositionPercent)
	}
}

func assertTraceReason(t *testing.T, rows []diagnostics.MotionTraceRow, reason string) {
	t.Helper()
	for _, row := range rows {
		if row.Reason == reason && row.Target != nil {
			return
		}
	}
	t.Fatalf("trace rows = %+v, want reason %q with target", rows, reason)
}

func assertTraceAnnotation(t *testing.T, rows []diagnostics.MotionTraceRow, reason string, annotation string) {
	t.Helper()
	for _, row := range rows {
		if row.Reason == reason && row.Annotation == annotation {
			return
		}
	}
	t.Fatalf("trace rows = %+v, want reason %q annotation %q", rows, reason, annotation)
}

func findRetargetTrace(t *testing.T, rows []diagnostics.MotionTraceRow, reason string) diagnostics.MotionTraceRetarget {
	t.Helper()
	for _, row := range rows {
		if row.Reason == reason && row.Retarget != nil {
			return *row.Retarget
		}
	}
	t.Fatalf("trace rows = %+v, want retarget reason %q", rows, reason)
	return diagnostics.MotionTraceRetarget{}
}

func hasTraceAnnotationPrefix(rows []diagnostics.MotionTraceRow, reason string, prefix string) bool {
	for _, row := range rows {
		if row.Reason == reason && strings.HasPrefix(row.Annotation, prefix) {
			return true
		}
	}
	return false
}

func lastPointsAdd(commands []transport.Command) *transport.AppendPointsCommand {
	for index := len(commands) - 1; index >= 0; index-- {
		if commands[index].PointsAdd != nil {
			return commands[index].PointsAdd
		}
	}
	return nil
}
