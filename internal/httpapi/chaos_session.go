package httpapi

import (
	"math"
	"math/rand"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	chaosLeadMillis      = int64(300)
	chaosChainLeadMillis = int64(80)
)

func buildChaosSession(
	physics motion.ChaoticPhysics,
	motionSettings config.MotionSettings,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) manualqueue.Session {
	return buildChaosSessionFromPosition(physics, motionSettings, hardwareSafetyLock, rng, -1)
}

func buildChaosSessionFromPosition(
	physics motion.ChaoticPhysics,
	motionSettings config.MotionSettings,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) manualqueue.Session {
	waypoints := motion.GenerateStrokeWaypointsFromPosition(
		physics,
		motion.EstimateChatMotionDurationMS(physics),
		hardwareSafetyLock,
		rng,
		continueFrom,
	)
	lead := chaosLeadMillis
	if continueFrom >= 0 {
		lead = chaosChainLeadMillis
	}
	logMotionSessionTrace("buildChaosSessionFromPosition", physics, waypoints, lead, continueFrom, motionSettings)
	return chaosSessionFromWaypoints(waypoints, motionSettings, lead)
}

func buildChaosSessionForDuration(
	physics motion.ChaoticPhysics,
	motionSettings config.MotionSettings,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	targetDurationMillis int64,
) manualqueue.Session {
	return buildChaosSessionForDurationFromPosition(
		physics,
		motionSettings,
		hardwareSafetyLock,
		rng,
		targetDurationMillis,
		-1,
	)
}

func buildChaosSessionForDurationFromPosition(
	physics motion.ChaoticPhysics,
	motionSettings config.MotionSettings,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	targetDurationMillis int64,
	continueFrom int,
) manualqueue.Session {
	waypoints := motion.GenerateProceduralStreamFromPosition(
		physics,
		targetDurationMillis,
		hardwareSafetyLock,
		rng,
		continueFrom,
	)
	lead := chaosLeadMillis
	if continueFrom >= 0 {
		lead = chaosChainLeadMillis
	}
	logMotionSessionTrace("buildChaosSessionForDurationFromPosition", physics, waypoints, lead, continueFrom, motionSettings)
	return chaosSessionFromWaypoints(waypoints, motionSettings, lead)
}

// buildChaosSessionForDurationFromBlend appends a Hermite crossfade from live device state.
func buildChaosSessionForDurationFromBlend(
	physics motion.ChaoticPhysics,
	motionSettings config.MotionSettings,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	targetDurationMillis int64,
	from motion.MotionBlendState,
) manualqueue.Session {
	waypoints := motion.GenerateProceduralStreamFromBlend(
		physics,
		targetDurationMillis,
		hardwareSafetyLock,
		rng,
		from,
	)
	logMotionSessionTrace("buildChaosSessionForDurationFromBlend", physics, waypoints, 0, int(math.Round(from.Position)), motionSettings)
	session := chaosSessionFromWaypoints(waypoints, motionSettings, 0)
	session.Continuous = true
	return session
}

func estimateBlendStateFromPlayer(player *manualqueue.Player) motion.MotionBlendState {
	snap := player.Snapshot()
	actions := player.Actions()
	playhead := snap.PlayheadMS
	pos := manualqueue.PositionAt(actions, playhead)
	lookback := playhead - 80
	if lookback < 0 {
		lookback = 0
	}
	posBefore := manualqueue.PositionAt(actions, lookback)
	elapsed := float64(playhead-lookback) / 1000.0
	vel := 0.0
	if elapsed > 0 {
		vel = (pos - posBefore) / elapsed
	}
	return motion.MotionBlendState{
		Position: pos,
		Velocity: vel,
	}
}

func logMotionSessionTrace(
	stage string,
	physics motion.ChaoticPhysics,
	waypoints []motion.ChaosWaypoint,
	lead int64,
	continueFrom int,
	motionSettings config.MotionSettings,
) {
	trace := motion.SummarizeMotionTrace(physics, waypoints, lead, continueFrom)
	data := map[string]any{
		"stage":              stage,
		"trace":              trace,
		"stroke_min_percent": motionSettings.StrokeMinPercent,
		"stroke_max_percent": motionSettings.StrokeMaxPercent,
		"hardware_safety":    motionSettings.HardwareSafetyLock,
	}
	if len(waypoints) > 0 {
		preview := make([]map[string]any, 0, 4)
		for i := 0; i < len(waypoints) && i < 4; i++ {
			preview = append(preview, map[string]any{
				"pos": waypoints[i].Position,
				"dt":  waypoints[i].TimeDelta,
			})
		}
		data["waypoint_preview"] = preview
	}
	motion.MotionDebugLog("M1", "chaos_session.go:"+stage, "session trace", data)
}

func chaosSessionFromWaypoints(
	waypoints []motion.ChaosWaypoint,
	motionSettings config.MotionSettings,
	leadMillis int64,
) manualqueue.Session {
	points := motion.ChaosWaypointsToTimedPoints(waypoints, leadMillis)
	return manualqueue.Session{
		Actions:    timedPointsToManualActions(points),
		Points:     points,
		DurationMS: motion.ChaosWaypointsDurationMS(waypoints, leadMillis),
		StrokeMin:  motionSettings.StrokeMinPercent,
		StrokeMax:  motionSettings.StrokeMaxPercent,
	}
}

func timedPointsToManualActions(points []transport.TimedPoint) []manualqueue.Action {
	actions := make([]manualqueue.Action, len(points))
	for index, point := range points {
		actions[index] = manualqueue.Action{
			At:  int(point.TimeMillis),
			Pos: point.PositionPercent,
		}
	}
	return actions
}
