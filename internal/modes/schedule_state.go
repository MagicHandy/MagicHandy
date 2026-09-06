package modes

import "time"

type autopilotWork uint8

const (
	waitForRetry autopilotWork = iota
	applyPlannedMotion
	planNextMotion
	maintainMotionAndSpeech
)

// nextAutopilotWork selects work from one protected schedule observation. It
// changes no state, samples no randomness, and performs no engine/model calls.
// The manager performs the chosen transition with its generation checks.
func (s *motionScheduleState) nextAutopilotWork(now time.Time) autopilotWork {
	if now.Before(s.nextRetry) {
		return waitForRetry
	}
	if s.pending != nil && !now.Before(s.deadline) {
		return applyPlannedMotion
	}
	if s.pending == nil && (s.planAt.IsZero() || !now.Before(s.planAt)) {
		return planNextMotion
	}
	return maintainMotionAndSpeech
}

func postpone(clock *time.Time, by time.Duration) {
	if !clock.IsZero() {
		*clock = clock.Add(by)
	}
}

func (s *motionScheduleState) postpone(by time.Duration) {
	postpone(&s.deadline, by)
	postpone(&s.driftAt, by)
	postpone(&s.planAt, by)
	for i := range s.swayPoints {
		s.swayPoints[i].at = s.swayPoints[i].at.Add(by)
	}
}

func (s *speechScheduleState) postpone(by time.Duration) {
	postpone(&s.deadline, by)
	postpone(&s.fallbackAt, by)
}

func (s *motionHistoryState) postpone(by time.Duration) {
	postpone(&s.speedChangedAt, by)
	postpone(&s.phraseChangedAt, by)
	postpone(&s.arc.startedAt, by)
}
