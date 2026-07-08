package motion

import (
	"encoding/json"
	"math"
	"os"
	"time"
)

const motionDebugLogPath = `c:\dev\git\Handy\debug-d9c091.log`

// MotionTraceSummary compares AI physics intent with generated waypoint stats.
type MotionTraceSummary struct {
	Regiao            string  `json:"regiao"`
	TipoBatida        string  `json:"tipo_batida"`
	Velocidade        int     `json:"velocidade"`
	Intensidade       int     `json:"intensidade"`
	RequestedAtrasoMS int     `json:"requested_atraso_ms"`
	EffectiveAtrasoMS int     `json:"effective_atraso_ms"`
	ZoneMin           int     `json:"zone_min"`
	ZoneMax           int     `json:"zone_max"`
	PosMin            int     `json:"pos_min"`
	PosMax            int     `json:"pos_max"`
	PointsInZonePct   float64 `json:"points_in_zone_pct"`
	PointCount        int     `json:"point_count"`
	AvgDeltaMS        int     `json:"avg_delta_ms"`
	MinDeltaMS        int     `json:"min_delta_ms"`
	MaxDeltaMS        int     `json:"max_delta_ms"`
	DurationMS        int     `json:"duration_ms"`
	LeadMS            int64   `json:"lead_ms"`
	ContinueFrom      int     `json:"continue_from,omitempty"`
	PositionGaps      int     `json:"position_gaps_gt15"`
	MaxPositionStep   int     `json:"max_position_step"`
}

// SummarizeMotionTrace builds calibration metrics for debug validation.
func SummarizeMotionTrace(
	physics ChaoticPhysics,
	waypoints []ChaosWaypoint,
	leadMillis int64,
	continueFrom int,
) MotionTraceSummary {
	region, ok := chaosRegionRange(physics.Regiao)
	if !ok {
		region = regionRange{Min: 0, Max: 100}
	}

	summary := MotionTraceSummary{
		Regiao:            physics.Regiao,
		TipoBatida:        physics.TipoBatida,
		Velocidade:        physics.Velocidade,
		Intensidade:       physics.Intensidade,
		RequestedAtrasoMS: physics.AtrasoMS,
		EffectiveAtrasoMS: ResolveAtrasoMS(physics),
		ZoneMin:           region.Min,
		ZoneMax:           region.Max,
		PointCount:        len(waypoints),
		LeadMS:            leadMillis,
		ContinueFrom:      continueFrom,
		DurationMS:        ChaosWaypointsDurationMS(waypoints, leadMillis),
	}
	if len(waypoints) == 0 {
		return summary
	}

	inZone := 0
	posMin, posMax := waypoints[0].Position, waypoints[0].Position
	deltaTotal := 0
	deltaCount := 0
	minDelta, maxDelta := 0, 0
	gaps := 0
	maxStep := 0
	prevPos := waypoints[0].Position
	for i, wp := range waypoints {
		if wp.Position < posMin {
			posMin = wp.Position
		}
		if wp.Position > posMax {
			posMax = wp.Position
		}
		if wp.Position >= region.Min && wp.Position <= region.Max {
			inZone++
		}
		if i > 0 {
			step := absIntValue(wp.Position - prevPos)
			if step > maxStep {
				maxStep = step
			}
			delta := wp.TimeDelta
			deltaTotal += delta
			deltaCount++
			if deltaCount == 1 || delta < minDelta {
				minDelta = delta
			}
			if delta > maxDelta {
				maxDelta = delta
			}
			if absIntValue(wp.Position-prevPos) > 15 {
				gaps++
			}
		}
		prevPos = wp.Position
	}
	summary.PosMin = posMin
	summary.PosMax = posMax
	if len(waypoints) > 0 {
		summary.PointsInZonePct = math.Round(float64(inZone)*1000/float64(len(waypoints))) / 10
	}
	if deltaCount > 0 {
		summary.AvgDeltaMS = deltaTotal / deltaCount
		summary.MinDeltaMS = minDelta
		summary.MaxDeltaMS = maxDelta
	}
	summary.PositionGaps = gaps
	summary.MaxPositionStep = maxStep
	return summary
}

// #region agent log
func MotionDebugLog(hypothesisID, location, message string, data map[string]any) {
	payload := map[string]any{
		"sessionId":    "d9c091",
		"hypothesisId": hypothesisID,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
		"runId":        "motion-calibrate",
	}
	line, err := json.Marshal(payload)
	if err != nil {
		return
	}
	f, err := os.OpenFile(motionDebugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
}

// #endregion
