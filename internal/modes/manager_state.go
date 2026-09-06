package modes

import (
	"context"
	"math/rand"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// State records have explicit owners and are all protected by Manager.mu.
// They add no locks or goroutines: a transition across records stays atomic.
type modeLoopState struct {
	mode            string
	cancel          context.CancelFunc
	done            chan struct{}
	generation      uint64
	wasPaused       bool
	operationID     uint64
	operationMode   string
	operationCancel context.CancelFunc
}

type userControlState struct {
	// paused closes the gap while Engine.Pause is doing transport I/O but has
	// not yet published its final paused snapshot.
	paused          bool
	intentID        uint64
	pauseConfirmed  uint64
	resumeConfirmed uint64
	pending         map[uint64]struct{}
	stopped         bool
}

type chatRecoveryState struct {
	target     *motion.MotionTarget
	keepalive  bool
	pending    bool
	activity   bool
	activityID uint64
	version    uint64
}

func (s *chatRecoveryState) beginActivity() uint64 {
	s.activityID++
	s.activity = true
	return s.activityID
}

func (s *chatRecoveryState) completeActivity(id uint64) {
	if id == s.activityID {
		s.activity = false
	}
}

type motionScheduleState struct {
	planner          *Planner
	segment          Segment
	pattern          *motion.PatternDefinition
	deadline         time.Time
	driftAt          time.Time
	driftDone        bool
	nextRetry        time.Time
	segmentIdx       int
	planAt           time.Time
	pending          *segmentChoice
	lastDecisionTime time.Duration
	cadenceRNG       *rand.Rand
	swayRNG          *rand.Rand
	// swayPoints is the remaining intra-segment speed schedule, in time order.
	swayPoints []swayPoint
}

type speechScheduleState struct {
	lastSay    string
	deadline   time.Time
	waitingID  string
	fallbackAt time.Time
	nextTiming TimingPreference
	cadenceRNG *rand.Rand
}

type motionHistoryState struct {
	recentPatternIDs    []string
	recentPositionBands []PositionBand
	// Speed age and direction distinguish deliberate plateaus from accidents
	// in the session facts handed to the model.
	speedChangedAt time.Time
	previousSpeed  int
	// Phrase history excludes speed and decision horizon so small pace nudges
	// cannot make a long-repeated shape look new to the model.
	currentPhrase            Segment
	currentPerceptual        *motion.PerceptualSummary
	phraseChangedAt          time.Time
	decisionsAtCurrentPhrase int
	consecutiveHolds         int
	arc                      arcState
}

type modeEventState struct {
	lastEvent      string
	lastEventAt    time.Time
	decisionSource string
}
