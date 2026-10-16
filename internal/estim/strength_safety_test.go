package estim

import (
	"testing"
	"time"
)

func TestReviewNoQueuedIncreaseAfterLostConfirmation(t *testing.T) {
	c := NewStrengthController(20, 20)
	c.Report(StrengthReport{A: 5, B: 0})
	now := time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC)
	if err := c.Step(ChannelA, 1); err != nil {
		t.Fatal(err)
	}
	first, _, seq := c.Next(now)
	if first.Mode != StrengthIncrease || seq == 0 {
		t.Fatal("fixture did not issue first increase")
	}
	if err := c.Step(ChannelA, 1); err != nil {
		t.Fatal(err)
	}
	// No B1 arrives for the first increase.
	next, _, seq := c.Next(now.Add(2 * time.Second))
	t.Logf("after lost acknowledgment: known=%v next=%+v sequence=%d", c.Snapshot().Known, next, seq)
	if next.Mode == StrengthIncrease {
		t.Fatal("sent a queued increase while actual strength was unknown")
	}
}

func TestReviewZeroTimeoutPreservesNewerPhysicalReport(t *testing.T) {
	c := NewStrengthController(20, 20)
	now := time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC)
	c.RequestZero()
	c.Next(now)
	// An already-zero device has no matching B1; a later physical adjustment
	// reports its actual strength with sequence zero.
	c.Report(StrengthReport{Sequence: 0, A: 5, B: 0})
	c.Next(now.Add(2 * time.Second))
	snapshot := c.Snapshot()
	t.Logf("actual device report A=5, controller after timeout=%+v", snapshot)
	if snapshot.Known && snapshot.A != 5 {
		t.Fatal("zero timeout overwrote a newer device report with an invented confirmed zero")
	}
}
