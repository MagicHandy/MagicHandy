package estim

import (
	"errors"
	"testing"
	"time"
)

var handshakeStart = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func confirmedController(t *testing.T, a, b int) *StrengthController {
	t.Helper()
	controller := NewStrengthController(30, 30)
	controller.Report(StrengthReport{A: a, B: b})
	return controller
}

func TestStrengthIncreaseNeedsAConfirmedDeviceValue(t *testing.T) {
	controller := NewStrengthController(30, 30)
	if err := controller.Step(ChannelA, 1); !errors.Is(err, ErrStrengthUnconfirmed) {
		t.Fatalf("Step(+1) before any report = %v, want ErrStrengthUnconfirmed", err)
	}
	if err := controller.Step(ChannelA, -1); err != nil {
		t.Fatalf("lowering must stay available before a report: %v", err)
	}
	changeA, _, sequence := controller.Next(handshakeStart)
	if changeA != (StrengthChange{Mode: StrengthDecrease, Value: 1}) || sequence == 0 {
		t.Fatalf("Next() = %+v seq %d, want a sequenced decrease", changeA, sequence)
	}
}

func TestStrengthStepsBatchAndWaitForTheReport(t *testing.T) {
	controller := confirmedController(t, 4, 0)
	for range 3 {
		if err := controller.Step(ChannelA, 1); err != nil {
			t.Fatal(err)
		}
	}
	changeA, changeB, sequence := controller.Next(handshakeStart)
	if changeA != (StrengthChange{Mode: StrengthIncrease, Value: 3}) || changeB.Mode != StrengthKeep || sequence != 1 {
		t.Fatalf("Next() = %+v %+v seq %d", changeA, changeB, sequence)
	}
	if err := controller.Step(ChannelA, 1); err != nil {
		t.Fatal(err)
	}
	if changeA, _, sequence := controller.Next(handshakeStart.Add(100 * time.Millisecond)); changeA.Mode != StrengthKeep || sequence != 0 {
		t.Fatalf("a second change was sent while one was in flight: %+v seq %d", changeA, sequence)
	}
	controller.Report(StrengthReport{Sequence: 1, A: 7})
	changeA, _, sequence = controller.Next(handshakeStart.Add(200 * time.Millisecond))
	if changeA != (StrengthChange{Mode: StrengthIncrease, Value: 1}) || sequence != 2 {
		t.Fatalf("queued step after the report = %+v seq %d", changeA, sequence)
	}
}

func TestStrengthNeverExceedsTheCeiling(t *testing.T) {
	controller := confirmedController(t, 28, 0)
	if err := controller.Step(ChannelA, 5); err != nil {
		t.Fatal(err)
	}
	if err := controller.Step(ChannelA, 1); !errors.Is(err, ErrStrengthCeiling) {
		t.Fatalf("Step past the ceiling = %v, want ErrStrengthCeiling", err)
	}
	changeA, _, _ := controller.Next(handshakeStart)
	if changeA != (StrengthChange{Mode: StrengthIncrease, Value: 2}) {
		t.Fatalf("increase was not clamped to the ceiling: %+v", changeA)
	}
	controller.Report(StrengthReport{Sequence: 1, A: 30})
	controller.SetCeilings(20, 30)
	if err := controller.Step(ChannelA, 1); !errors.Is(err, ErrStrengthCeiling) {
		t.Fatalf("Step above a lowered ceiling = %v, want ErrStrengthCeiling", err)
	}
}

func TestStrengthZeroPreemptsQueuedAndInFlightChanges(t *testing.T) {
	controller := confirmedController(t, 10, 12)
	_ = controller.Step(ChannelA, 2)
	controller.Next(handshakeStart)
	_ = controller.Step(ChannelB, 3)
	controller.RequestZero()
	changeA, changeB, sequence := controller.Next(handshakeStart.Add(50 * time.Millisecond))
	set := StrengthChange{Mode: StrengthSet, Value: 0}
	if changeA != set || changeB != set || sequence == 0 {
		t.Fatalf("zero = %+v %+v seq %d", changeA, changeB, sequence)
	}
	if !controller.Pending() {
		t.Fatal("the zero should await confirmation")
	}
	// An unchanged value produces no B1. A successful write establishes an
	// explicitly commanded baseline; timeout alone does not establish a value.
	controller.ZeroWritten(sequence)
	controller.Next(handshakeStart.Add(2 * time.Second))
	if state := controller.Snapshot(); !state.Known || state.Source != StrengthZeroCommand || state.A != 0 || state.B != 0 || state.Pending {
		t.Fatalf("after the zero timeout = %+v", state)
	}
}

func TestStrengthZeroTimeoutWithoutSuccessfulWriteStaysUnknown(t *testing.T) {
	controller := confirmedController(t, 10, 12)
	controller.RequestZero()
	controller.Next(handshakeStart)
	controller.Next(handshakeStart.Add(2 * time.Second))
	if state := controller.Snapshot(); state.Known || state.Source != StrengthUnknown {
		t.Fatalf("timeout invented a zero confirmation: %+v", state)
	}
	if err := controller.Step(ChannelA, 1); !errors.Is(err, ErrStrengthUnconfirmed) {
		t.Fatalf("unconfirmed zero allowed an increase: %v", err)
	}
}

func TestStrengthZeroWriteNeverOverwritesANewerReport(t *testing.T) {
	for _, matching := range []bool{false, true} {
		controller := NewStrengthController(30, 30)
		controller.RequestZero()
		_, _, sequence := controller.Next(handshakeStart)
		report := StrengthReport{A: 5}
		if matching {
			report.Sequence = sequence
			report.A = 0
		}
		controller.Report(report)
		controller.ZeroWritten(sequence)
		controller.Next(handshakeStart.Add(2 * time.Second))
		if state := controller.Snapshot(); !state.Known || state.Source != StrengthReported || state.A != report.A {
			t.Fatalf("matching=%v: write/timeout overwrote device evidence: %+v", matching, state)
		}
	}
}

func TestStrengthUnconfirmedRelativeChangeBlocksIncreases(t *testing.T) {
	controller := confirmedController(t, 5, 5)
	_ = controller.Step(ChannelA, 1)
	controller.Next(handshakeStart)
	controller.Next(handshakeStart.Add(2 * time.Second))
	if state := controller.Snapshot(); state.Known {
		t.Fatalf("a lost report must leave the strength unknown: %+v", state)
	}
	if err := controller.Step(ChannelA, 1); !errors.Is(err, ErrStrengthUnconfirmed) {
		t.Fatalf("Step(+1) after a lost report = %v", err)
	}
	controller.Report(StrengthReport{A: 6, B: 5})
	if err := controller.Step(ChannelA, 1); err != nil {
		t.Fatalf("a device report (sequence 0) must re-confirm the strength: %v", err)
	}
}

func TestStrengthSequenceSkipsZero(t *testing.T) {
	controller := confirmedController(t, 0, 0)
	seen := map[int]bool{}
	for step := range 40 {
		_ = controller.Step(ChannelB, 1)
		_, _, sequence := controller.Next(handshakeStart.Add(time.Duration(step) * time.Second))
		if sequence < 1 || sequence > MaxSequence {
			t.Fatalf("sequence %d outside 1..15", sequence)
		}
		seen[sequence] = true
		controller.Report(StrengthReport{Sequence: sequence, B: 0})
	}
	if len(seen) != MaxSequence {
		t.Fatalf("sequences cycled through %d values, want %d", len(seen), MaxSequence)
	}
}

func TestStrengthResetKeepsCeilings(t *testing.T) {
	controller := confirmedController(t, 9, 9)
	controller.Reset()
	if state := controller.Snapshot(); state.Known || state.CeilingA != 30 || state.CeilingB != 30 {
		t.Fatalf("Reset() = %+v", state)
	}
}
