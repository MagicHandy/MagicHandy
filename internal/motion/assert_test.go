package motion

import (
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
		case transport.CommandKindHSPPlay:
			playCount++
		case transport.CommandKindStop:
			stopCount++
		}
	}
	if playCount != 1 || stopCount != 0 {
		t.Fatalf("commands = %+v, want one play and no regular stop before explicit stop", commands)
	}
}

func assertReversePointMapping(t *testing.T, commands []transport.Command, sample *MotionSample) {
	t.Helper()
	if sample == nil {
		t.Fatal("last semantic sample is missing")
	}
	add := lastHSPAdd(commands)
	if add == nil || len(add.Points) == 0 {
		t.Fatalf("commands = %+v, want HSP add points", commands)
	}
	got := add.Points[len(add.Points)-1].PositionPercent
	want := 100 - sample.PositionPercent
	if got != want {
		t.Fatalf("last transport point = %d, want reverse of semantic sample %d", got, sample.PositionPercent)
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

func lastHSPAdd(commands []transport.Command) *transport.HSPAddCommand {
	for index := len(commands) - 1; index >= 0; index-- {
		if commands[index].HSPAdd != nil {
			return commands[index].HSPAdd
		}
	}
	return nil
}
