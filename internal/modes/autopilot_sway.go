package modes

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// Intra-segment sway replaces the old fixed midpoint drift.
//
// The old behavior fired one speed step at exactly duration/2, and only when a
// segment carried DriftToSpeedPercent — which only the Freestyle planner sets.
// Autopilot's model-chosen segments therefore never had any variation inside a
// segment at all, and after cadence windows grew to 20-60s (up to 120s on
// Steady) a target could hold perfectly constant for a minute or more.
//
// Sway samples a few speed-only waypoints across the segment interior instead:
//
//   - Speed only, inside the current pattern and area. A sway can never change
//     pattern or focus, so it cannot smuggle in a semantic change and it does not
//     disturb the recognizable feel that longer segments exist to establish.
//   - Offsets are sampled, not fixed at the midpoint, so the texture is not
//     metronomic.
//   - Count scales with segment length, which self-balances against the cadence
//     preset: a 10s Dynamic segment has no room and gets none (it is already
//     changing constantly), while a 120s Steady segment gets the most.
//   - Amplitude is a fraction of the user's own speed band and every waypoint is
//     clamped inside it. Sway widens nothing.
const (
	// swaySecondsPerPoint earns one waypoint per this much segment length.
	swaySecondsPerPoint = 20
	// maxSwayPoints bounds the retarget budget on very long segments.
	maxSwayPoints = 3
	// swayEdgeGuard keeps waypoints clear of both segment boundaries so a sway
	// never lands on top of a semantic change.
	swayEdgeGuard = 4 * time.Second
	// swayMinSpacing is the least time between consecutive waypoints.
	swayMinSpacing = 6 * time.Second
	// swayBandPercentNormal and swayBandPercentRestless are the share of the
	// user's speed band one waypoint may move through.
	swayBandPercentNormal   = 14
	swayBandPercentRestless = 26
	// swayMinAmplitude keeps a waypoint perceptible when the user's band is
	// narrow. It is still clamped inside the band afterwards.
	swayMinAmplitude = 2
	// swayCadenceSeedSalt keeps sway sampling independent of the two cadence
	// clocks so identical seeds do not correlate timing with amplitude.
	swayCadenceSeedSalt = int64(0x2545f4914f6cdd1d)
)

func swayNote(speedPercent int, remaining int) string {
	return fmt.Sprintf("speed=%d%% remaining=%d", speedPercent, remaining)
}

// swayPoint is one scheduled speed-only adjustment inside the current segment.
type swayPoint struct {
	at           time.Time
	speedPercent int
}

// planSwayLocked builds the waypoint schedule for a freshly armed segment.
// Callers hold the lock.
func (m *Manager) planSwayLocked(
	now time.Time,
	duration time.Duration,
	choice segmentChoice,
) []swayPoint {
	if choice.segment.PatternID == "" || choice.segment.SpeedPercent <= 0 {
		return nil
	}
	allowed := m.swayAllowanceLocked(duration, choice.variability)
	if allowed <= 0 {
		return nil
	}
	settings := m.options.Settings()
	low, high := settings.SpeedMinPercent, settings.SpeedMaxPercent
	if low > high {
		low, high = high, low
	}
	if high <= 0 {
		return nil
	}
	amplitude := swayAmplitude(low, high, choice.variability)
	if amplitude <= 0 {
		return nil
	}

	m.ensureSwayRNGLocked()
	interior := duration - 2*swayEdgeGuard
	if interior <= 0 {
		return nil
	}
	// Evenly divided slots with a sampled offset inside each keeps the points
	// spread across the segment while still landing at unpredictable moments.
	slot := interior / time.Duration(allowed)
	if slot < swayMinSpacing {
		return nil
	}
	points := make([]swayPoint, 0, allowed)
	for index := 0; index < allowed; index++ {
		base := swayEdgeGuard + slot*time.Duration(index)
		jitter := time.Duration(0)
		if span := int64(slot - swayMinSpacing/2); span > 0 {
			jitter = time.Duration(m.swayRNG.Int63n(span))
		}
		delta := m.swayRNG.Intn(2*amplitude+1) - amplitude
		if delta == 0 {
			// A zero delta would be a no-op retarget, which is exactly what this
			// change set removed from the hold path. Nudge it off zero instead.
			delta = amplitude
			if m.swayRNG.Intn(2) == 0 {
				delta = -amplitude
			}
		}
		speed := clampInt(choice.segment.SpeedPercent+delta, low, high)
		if speed == choice.segment.SpeedPercent {
			continue
		}
		points = append(points, swayPoint{at: now.Add(base + jitter), speedPercent: speed})
	}
	return points
}

// swayAllowanceLocked converts segment length and the model's variability
// category into a waypoint count. Callers hold the lock.
func (m *Manager) swayAllowanceLocked(duration time.Duration, variability VariabilityPreference) int {
	if normalizeVariability(variability) == VariabilitySettled {
		return 0
	}
	byLength := int(duration / (swaySecondsPerPoint * time.Second))
	if byLength > maxSwayPoints {
		byLength = maxSwayPoints
	}
	if byLength <= 0 {
		return 0
	}
	if normalizeVariability(variability) == VariabilityRestless {
		return byLength
	}
	// Normal takes roughly half the earned allowance, but never drops a segment
	// that earned one to zero.
	allowance := (byLength + 1) / 2
	if allowance < 1 {
		allowance = 1
	}
	return allowance
}

func swayAmplitude(low int, high int, variability VariabilityPreference) int {
	band := high - low
	if band <= 0 {
		return 0
	}
	share := swayBandPercentNormal
	if normalizeVariability(variability) == VariabilityRestless {
		share = swayBandPercentRestless
	}
	amplitude := band * share / 100
	if amplitude < swayMinAmplitude {
		amplitude = swayMinAmplitude
	}
	if amplitude > band {
		amplitude = band
	}
	return amplitude
}

// dueSway pops the first waypoint whose moment has arrived. It pops on read
// rather than on success: a waypoint that cannot be applied is texture that is
// no longer wanted, and retrying it would let one failing adjustment starve the
// speech clock behind it.
func (m *Manager) dueSway(now time.Time, generation uint64) (swayPoint, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mode != ModeAutopilot || m.generation != generation || m.userStopped ||
		m.chatTargetPending || len(m.swayPoints) == 0 {
		return swayPoint{}, false
	}
	if now.Before(m.swayPoints[0].at) {
		return swayPoint{}, false
	}
	point := m.swayPoints[0]
	m.swayPoints = m.swayPoints[1:]
	return point, true
}

// applyDueSway adjusts speed inside the live segment without touching the
// segment deadline: a sway is texture, not a reconsideration boundary.
func (m *Manager) applyDueSway(
	ctx context.Context,
	engine Engine,
	generation uint64,
	point swayPoint,
) {
	operationCtx, finish, ok := m.beginStartOperation(ctx, ModeAutopilot, generation, 0)
	if !ok {
		return
	}
	defer finish()

	m.mu.Lock()
	segment := m.segment
	pattern := m.pattern
	m.mu.Unlock()
	if segment.PatternID == "" {
		return
	}
	segment.SpeedPercent = point.speedPercent
	target := segment.Target(modeLabel(ModeAutopilot), ModeAutopilot)
	target.Pattern = pattern
	if _, err := engine.ApplyTarget(operationCtx, target, "autopilot_sway"); err != nil {
		if operationCtx.Err() == nil {
			m.trace(ModeAutopilot, "sway_failed", nil, err.Error())
		}
		return
	}
	m.mu.Lock()
	if m.mode == ModeAutopilot && m.generation == generation {
		m.segment.SpeedPercent = point.speedPercent
		m.speedChangedAt = m.options.Now()
	}
	remaining := len(m.swayPoints)
	m.mu.Unlock()
	m.trace(ModeAutopilot, "autopilot_sway", nil,
		swayNote(point.speedPercent, remaining))
}

func (m *Manager) ensureSwayRNGLocked() {
	if m.planner == nil {
		m.planner = NewPlanner(m.options.Seed)
	}
	if m.swayRNG == nil {
		//nolint:gosec // Reproducible cadence variation, never security material.
		m.swayRNG = rand.New(rand.NewSource(m.planner.Seed() ^ swayCadenceSeedSalt))
	}
}
