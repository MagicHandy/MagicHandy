package motion

import "context"

// editHandoff identifies the accepted path immediately before the first
// uncommitted point. Several edits at that cursor all splice into this same
// path, so discarded targets cannot add transitions or move the handoff.
type editHandoff struct {
	at         int64
	plan       MotionPlan
	transition *planTransition
}

func (e *Engine) editPathLocked(playback int64) (MotionPlan, *planTransition) {
	if pending := e.pendingEdit; pending != nil && pending.at == e.nextSampleMillis {
		return pending.plan, pruneTransitionHistory(pending.transition, playback)
	}
	return e.plan, pruneTransitionHistory(e.transition, playback)
}

// installEditLocked keeps the accepted trajectory observable until the device
// reaches the handoff, including for a pace change that needs no spatial blend.
func (e *Engine) installEditLocked(previous MotionPlan, previousTransition *planTransition, next *MotionPlan, handoff, estimatedMillis int64) bool {
	bridgeInserted := transitionRequired(previous, previousTransition, *next, handoff)
	if bridgeInserted {
		e.transition = layeredTurnTransition(previous, previousTransition, next, handoff)
		if e.transition == nil {
			e.transition = newPlanTransition(previous, previousTransition, handoff)
		}
	} else if handoff > estimatedMillis && next.Target.Media == nil {
		e.transition = &planTransition{previousPlan: previous, previousTransition: previousTransition, startMillis: handoff, endMillis: handoff}
	} else {
		e.transition = nil
	}
	e.pendingEdit = &editHandoff{at: handoff, plan: previous, transition: previousTransition}
	e.plan = *next
	return bridgeInserted
}

func (e *Engine) dispatchInteractiveEdit(ctx context.Context, epoch uint64, reason string) error {
	e.mu.Lock()
	if err := e.validateRunLocked(epoch); err != nil {
		e.mu.Unlock()
		return err
	}
	// The ordinary refill owns the next chunk when the device already has its
	// required lead plus a full frame. A burst thus commits at most one extra
	// frame beyond the normal refill horizon, regardless of its edit count.
	lead := e.nextSampleMillis - e.estimatedPlaybackMillisLocked(e.now())
	window := e.leadMillisLocked() + int64(e.chunkSize)*e.sampleInterval.Milliseconds()
	deferToRefill := e.plan.Target.Media == nil && lead > window
	e.mu.Unlock()
	if deferToRefill {
		return nil
	}
	return e.dispatchNextChunk(ctx, epoch, reason)
}
