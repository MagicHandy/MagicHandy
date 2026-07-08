package motion

import (
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// ChaosWaypointsToTimedPoints converts relative chaotic steps into absolute HSP
// stream timestamps. The first point is scheduled at leadMillis; each following
// point adds its TimeDelta to the running clock.
func ChaosWaypointsToTimedPoints(waypoints []ChaosWaypoint, leadMillis int64) []transport.TimedPoint {
	if len(waypoints) == 0 {
		return nil
	}
	if leadMillis < 0 {
		leadMillis = 0
	}

	points := make([]transport.TimedPoint, len(waypoints))
	elapsed := leadMillis
	for index, waypoint := range waypoints {
		if index > 0 {
			elapsed += int64(waypoint.TimeDelta)
		}
		pos := waypoint.Position
		if pos < 0 {
			pos = 0
		}
		if pos > 100 {
			pos = 100
		}
		points[index] = transport.TimedPoint{
			PositionPercent: pos,
			TimeMillis:      elapsed,
		}
	}
	return points
}

// ChaosWaypointsDurationMS returns the stream duration covered by the waypoint set.
func ChaosWaypointsDurationMS(waypoints []ChaosWaypoint, leadMillis int64) int {
	points := ChaosWaypointsToTimedPoints(waypoints, leadMillis)
	if len(points) == 0 {
		return 0
	}
	last := points[len(points)-1].TimeMillis
	if last < 0 {
		return 0
	}
	return int(last)
}
