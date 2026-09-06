package modes

import (
	"testing"
	"time"
)

func TestAutopilotWorkSelectionPreservesBoundaryPrecedence(t *testing.T) {
	now := time.Unix(100, 0)
	for _, test := range []struct {
		name     string
		schedule motionScheduleState
		want     autopilotWork
	}{
		{"retry delays even an overdue choice", motionScheduleState{nextRetry: now.Add(time.Second), pending: &segmentChoice{}, deadline: now.Add(-time.Second)}, waitForRetry},
		{"choice applies exactly at deadline", motionScheduleState{pending: &segmentChoice{}, deadline: now}, applyPlannedMotion},
		{"retry expiry admits overdue motion", motionScheduleState{nextRetry: now, pending: &segmentChoice{}, deadline: now.Add(-time.Second)}, applyPlannedMotion},
		{"queued choice waits without replanning", motionScheduleState{pending: &segmentChoice{}, deadline: now.Add(time.Second), planAt: now.Add(-time.Second)}, maintainMotionAndSpeech},
		{"unplanned motion plans immediately", motionScheduleState{}, planNextMotion},
		{"planning starts at its deadline", motionScheduleState{planAt: now}, planNextMotion},
		{"future plan leaves texture and speech available", motionScheduleState{planAt: now.Add(time.Second)}, maintainMotionAndSpeech},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.schedule.nextAutopilotWork(now); got != test.want {
				t.Fatalf("work = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPausePreservesRelativeClocksWithoutArmingAbsentWork(t *testing.T) {
	now := time.Unix(100, 0)
	motion := motionScheduleState{deadline: now, swayPoints: []swayPoint{{at: now.Add(time.Second)}}}
	speech := speechScheduleState{fallbackAt: now.Add(2 * time.Second)}
	history := motionHistoryState{speedChangedAt: now.Add(-time.Second), arc: arcState{startedAt: now.Add(-time.Minute)}}
	by := 750 * time.Millisecond
	motion.postpone(by)
	speech.postpone(by)
	history.postpone(by)
	if !motion.deadline.Equal(now.Add(by)) || !motion.swayPoints[0].at.Equal(now.Add(time.Second+by)) {
		t.Fatal("motion clocks drifted during pause")
	}
	if !speech.fallbackAt.Equal(now.Add(2*time.Second+by)) || !history.arc.startedAt.Equal(now.Add(-time.Minute+by)) {
		t.Fatal("speech or history clocks drifted during pause")
	}
	if !motion.planAt.IsZero() || !motion.driftAt.IsZero() || !speech.deadline.IsZero() || !history.phraseChangedAt.IsZero() {
		t.Fatal("pause armed a previously absent deadline")
	}
}
