package modes

import (
	"context"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// ReviewTicket identifies the mode intent that accepted a local choice. It
// does not reserve the mode: ordinary planning continues during the review.
type ReviewTicket struct {
	mode                          string
	generation, activity, version uint64
	segment                       int
}

func (m *Manager) reviewTicketLocked() ReviewTicket {
	return ReviewTicket{m.loop.mode, m.loop.generation, m.chat.activityID, m.chat.version, m.motion.segmentIdx}
}

// MotionReviewTicket captures admission after a local choice has been applied.
func (m *Manager) MotionReviewTicket() (ReviewTicket, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reviewTicketLocked(), m.reviewEligibleLocked()
}

func (m *Manager) reviewEligibleLocked() bool {
	return (m.loop.mode == "" || m.loop.mode == ModeChat || m.loop.mode == ModeAutopilot) &&
		!m.user.stopped && !m.user.paused && !m.chat.pending && !m.chat.activity
}

// ApplyMotionReview invalidates older mode work, then adopts the corrected
// semantic target for both keepalive and Autopilot. The caller still owns the
// atomic engine plan-ID check. There is no additional motion loop.
func (m *Manager) ApplyMotionReview(ctx context.Context, ticket ReviewTicket, target motion.MotionTarget,
	apply func(context.Context) (motion.ActiveMotionState, error),
) error {
	m.mu.Lock()
	if ctx.Err() != nil || !m.reviewEligibleLocked() || m.reviewTicketLocked() != ticket {
		m.mu.Unlock()
		return errors.New("motion review superseded by mode or chat activity")
	}
	m.chat.pending = true
	m.loop.generation++
	generation := m.loop.generation
	m.chat.pendingOwner = generation
	m.cancelOperationLocked()
	m.motion.pending = nil
	m.motion.swayPoints = nil
	operation, cancel := context.WithCancel(ctx)
	m.loop.operationID++
	id := m.loop.operationID
	m.loop.operationMode, m.loop.operationCancel = m.loop.mode, cancel
	m.mu.Unlock()
	// A newer chat activity can invalidate the generation without replacing
	// this handoff. Always release our own pending state, including when an
	// already committed send returns after its generation was superseded.
	defer m.CancelChatTarget(generation)
	defer func() {
		cancel()
		m.mu.Lock()
		if m.loop.operationID == id {
			m.loop.operationCancel, m.loop.operationMode = nil, ""
		}
		m.mu.Unlock()
	}()
	if _, err := apply(operation); err != nil {
		return err
	}
	if !m.notifyTarget(generation, target, true) {
		return errors.New("motion review superseded during handoff")
	}
	return nil
}

func (m *Manager) observeChoice(mode string, choice segmentChoice) {
	if mode != ModeAutopilot || m.options.ObserveChoice == nil || choice.reviewInput == nil {
		return
	}
	if engine := m.options.Current(); engine != nil {
		m.options.ObserveChoice(*choice.reviewInput, engine.Snapshot())
	}
}

func (m *Manager) inputForReview(input DecisionInput, choice segmentChoice) *DecisionInput {
	if m.options.ObserveChoice == nil || (m.options.ReviewEnabled != nil && !m.options.ReviewEnabled()) {
		return nil
	}
	if choice.source == "model" || (choice.source == "hold" && choice.note == "") {
		return &input
	}
	return nil
}
