package manualqueue

import (
	"sort"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// TimelineOptions controls funscript → HSP point conversion.
type TimelineOptions struct {
	StrokeMinPercent int
	StrokeMaxPercent int
	RemapStroke      bool
}

// ActionsToTimedPoints converts keyframes into HSP timed points.
func ActionsToTimedPoints(actions []Action, opts TimelineOptions) []transport.TimedPoint {
	if len(actions) == 0 {
		return nil
	}
	sorted := append([]Action(nil), actions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].At < sorted[j].At })
	points := make([]transport.TimedPoint, len(sorted))
	for i, action := range sorted {
		pos := action.Pos
		if pos < 0 {
			pos = 0
		}
		if pos > 100 {
			pos = 100
		}
		if opts.RemapStroke {
			pos = remapPosition(pos, opts.StrokeMinPercent, opts.StrokeMaxPercent)
		}
		points[i] = transport.TimedPoint{
			PositionPercent: pos,
			TimeMillis:      int64(action.At),
		}
	}
	return points
}

// DurationMS returns the script duration from the last keyframe.
func DurationMS(actions []Action) int {
	if len(actions) == 0 {
		return 0
	}
	maxAt := 0
	for _, action := range actions {
		if action.At > maxAt {
			maxAt = action.At
		}
	}
	return maxAt
}

// PositionAt interpolates position percent at elapsed ms.
func PositionAt(actions []Action, elapsedMS int) float64 {
	if len(actions) == 0 {
		return 50
	}
	if elapsedMS <= actions[0].At {
		return float64(actions[0].Pos)
	}
	last := actions[len(actions)-1]
	if elapsedMS >= last.At {
		return float64(last.Pos)
	}
	for i := 0; i < len(actions)-1; i++ {
		a := actions[i]
		b := actions[i+1]
		if elapsedMS < a.At || elapsedMS > b.At {
			continue
		}
		span := b.At - a.At
		if span <= 0 {
			return float64(b.Pos)
		}
		t := float64(elapsedMS-a.At) / float64(span)
		return float64(a.Pos) + t*float64(b.Pos-a.Pos)
	}
	return float64(last.Pos)
}

func remapPosition(pos, minPct, maxPct int) int {
	if minPct < 0 {
		minPct = 0
	}
	if maxPct > 100 {
		maxPct = 100
	}
	if minPct >= maxPct {
		return pos
	}
	return minPct + (pos*(maxPct-minPct)+50)/100
}
