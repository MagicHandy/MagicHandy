package modes

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestSkippedCheckInWaitsForSpeechCadenceWithoutPublication(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	var calls, publications atomic.Int32
	manager, err := NewManager(Options{
		Ensure: func(context.Context) (Engine, error) { return engine, nil }, Current: func() Engine { return engine },
		Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
		AutopilotSettings: func() config.AutopilotSettings {
			s := config.DefaultAutopilotSettings()
			s.MotionCadence, s.MotionMinSeconds, s.MotionMaxSeconds = config.AutopilotMotionCustom, 60, 60
			s.SpeechCadence, s.SpeechMinSeconds, s.SpeechMaxSeconds = config.AutopilotSpeechCustom, 20, 20
			s.AdaptiveMotionTiming, s.AdaptiveSpeechTiming = false, false
			return s
		},
		Now: clock.Now, Tick: 2 * time.Millisecond, Seed: 42,
		Decide: func(context.Context, DecisionInput) (Decision, error) {
			return Decision{Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30}, Next: TimingLater}, nil
		},
		DecideSpeech: func(context.Context, DecisionInput) (Decision, error) {
			calls.Add(1)
			return Decision{Hold: true, Abstain: true, Next: TimingLater}, nil
		},
		CanAnnounce: func() bool { return true },
		Announce:    func(context.Context, string) Announcement { publications.Add(1); return Announcement{Published: true} },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start(t.Context(), ModeAutopilot); err != nil {
		t.Fatal(err)
	}
	waitForAutonomousStart(t, manager, engine)
	clock.Advance(21 * time.Second)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		next := manager.speech.deadline
		waiting := manager.speech.waitingID
		manager.mu.Unlock()
		if calls.Load() == 1 && next.Equal(clock.Now().Add(20*time.Second)) {
			if waiting != "" || publications.Load() != 0 {
				t.Fatal("quiet turn published speech or waited for playback")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("quiet turn did not use normal cadence: calls=%d, published=%d", calls.Load(), publications.Load())
}
