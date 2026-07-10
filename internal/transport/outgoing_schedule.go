package transport

import (
	"context"
	"sync"
	"time"
)

const scheduleSparkWindowMS = int64(4000)

// OutgoingSchedule mirrors the HSP timed points committed to the device transport.
// The live visualizer reads this schedule with sync offset instead of guessing
// from heterogeneous engine/player snapshots.
type OutgoingSchedule struct {
	mu          sync.Mutex
	streamID    string
	points      []TimedPoint
	playWall    time.Time
	startTimeMS int64
	active      bool
}

// NewOutgoingSchedule returns an empty outgoing motion schedule.
func NewOutgoingSchedule() *OutgoingSchedule {
	return &OutgoingSchedule{}
}

// RecordingTransport wraps a transport and records HSP dispatch into a schedule.
type RecordingTransport struct {
	inner    Transport
	schedule *OutgoingSchedule
}

// NewRecordingTransport records HSP commands on schedule while delegating to inner.
func NewRecordingTransport(inner Transport, schedule *OutgoingSchedule) *RecordingTransport {
	if schedule == nil {
		schedule = NewOutgoingSchedule()
	}
	return &RecordingTransport{inner: inner, schedule: schedule}
}

func (t *RecordingTransport) Diagnostics() TransportDiagnostics {
	return t.inner.Diagnostics()
}

func (t *RecordingTransport) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	t.schedule.OnStop(command.Reason)
	return t.inner.Stop(ctx, command)
}

func (t *RecordingTransport) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	return t.inner.SetStrokeWindow(ctx, command)
}

func (t *RecordingTransport) AddHSP(ctx context.Context, command HSPAddCommand) (CommandResult, error) {
	t.schedule.OnAddHSP(command)
	return t.inner.AddHSP(ctx, command)
}

func (t *RecordingTransport) PlayHSP(ctx context.Context, command HSPPlayCommand) (CommandResult, error) {
	t.schedule.OnPlayHSP(command)
	return t.inner.PlayHSP(ctx, command)
}

// OnAddHSP appends dispatched timed points to the active schedule.
func (s *OutgoingSchedule) OnAddHSP(command HSPAddCommand) {
	if len(command.Points) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamID = command.StreamID
	for _, point := range command.Points {
		s.appendPointLocked(point)
	}
	s.active = true
}

// OnPlayHSP anchors wall-clock playback to the HSP stream start time.
func (s *OutgoingSchedule) OnPlayHSP(command HSPPlayCommand) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamID = command.StreamID
	s.playWall = time.Now()
	s.startTimeMS = command.StartTimeMillis
	s.active = true
}

// OnStop clears the outgoing schedule.
func (s *OutgoingSchedule) OnStop(_ string) {
	s.Reset()
}

// Reset clears the outgoing schedule.
func (s *OutgoingSchedule) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamID = ""
	s.points = nil
	s.playWall = time.Time{}
	s.startTimeMS = 0
	s.active = false
}

// RecordDirectMove records an HDSP-style immediate move for the visualizer.
func (s *OutgoingSchedule) RecordDirectMove(positionPct float64, durationMS int) {
	if durationMS <= 0 {
		durationMS = 66
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	streamMS := int64(0)
	if !s.playWall.IsZero() {
		streamMS = s.startTimeMS + now.Sub(s.playWall).Milliseconds()
	}
	fromPos := 50.0
	if len(s.points) > 0 {
		fromPos = float64(s.points[len(s.points)-1].PositionPercent)
	}
	if streamMS > 0 {
		s.appendPointLocked(TimedPoint{
			PositionPercent: int(fromPos + 0.5),
			TimeMillis:      streamMS,
		})
	}
	endMS := streamMS + int64(durationMS)
	s.appendPointLocked(TimedPoint{
		PositionPercent: int(positionPct + 0.5),
		TimeMillis:      endMS,
	})
	s.playWall = now
	s.startTimeMS = 0
	s.active = true
}

// Active reports whether the schedule has committed playback.
func (s *OutgoingSchedule) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active && len(s.points) > 0 && !s.playWall.IsZero()
}

// PositionAt returns the interpolated position for stream time with sync offset applied.
func (s *OutgoingSchedule) PositionAt(now time.Time, offsetMS int) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active || len(s.points) == 0 || s.playWall.IsZero() {
		return 0, false
	}
	streamMS := s.startTimeMS + now.Sub(s.playWall).Milliseconds() + int64(offsetMS)
	return interpolateTimedPoints(s.points, streamMS), true
}

// StreamElapsedMS returns the current stream timeline position without sync offset.
func (s *OutgoingSchedule) StreamElapsedMS(now time.Time) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active || s.playWall.IsZero() {
		return 0, false
	}
	return s.startTimeMS + now.Sub(s.playWall).Milliseconds(), true
}

// VisualSnapshot is the schedule view exposed to HTTP visual clients.
func (s *OutgoingSchedule) VisualSnapshot(now time.Time, offsetMS int) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active || len(s.points) == 0 || s.playWall.IsZero() {
		return nil
	}
	streamMS := s.startTimeMS + now.Sub(s.playWall).Milliseconds()
	visualMS := streamMS + int64(offsetMS)
	position := interpolateTimedPoints(s.points, visualMS)

	curve := scheduleCurveLocked(s.points, visualMS, scheduleSparkWindowMS)
	actions := make([]map[string]int, len(curve))
	for i, point := range curve {
		actions[i] = map[string]int{
			"at":  int(point.TimeMillis),
			"pos": point.PositionPercent,
		}
	}

	return map[string]any{
		"schedule_active":    true,
		"position_pct":         position,
		"live_position_pct":    position,
		"stream_elapsed_ms":    streamMS,
		"curve_actions":        actions,
		"curve_elapsed_ms":     int(visualMS),
		"curve_duration_ms":    int(scheduleDurationLocked(s.points)),
	}
}

func (s *OutgoingSchedule) appendPointLocked(point TimedPoint) {
	if len(s.points) > 0 {
		last := s.points[len(s.points)-1]
		if point.TimeMillis < last.TimeMillis {
			return
		}
		if point.TimeMillis == last.TimeMillis {
			s.points[len(s.points)-1] = point
			return
		}
	}
	s.points = append(s.points, point)
	if len(s.points) > 8192 {
		s.points = s.points[len(s.points)-8192:]
	}
}

func scheduleDurationLocked(points []TimedPoint) int64 {
	if len(points) == 0 {
		return 0
	}
	return points[len(points)-1].TimeMillis
}

func scheduleCurveLocked(points []TimedPoint, endMS int64, windowMS int64) []TimedPoint {
	if len(points) == 0 {
		return nil
	}
	startMS := endMS - windowMS
	if startMS < 0 {
		startMS = 0
	}
	out := make([]TimedPoint, 0, len(points))
	for _, point := range points {
		if point.TimeMillis < startMS {
			continue
		}
		out = append(out, TimedPoint{
			PositionPercent: point.PositionPercent,
			TimeMillis:      point.TimeMillis - startMS,
		})
	}
	return out
}

func interpolateTimedPoints(points []TimedPoint, elapsedMS int64) float64 {
	if len(points) == 0 {
		return 50
	}
	if elapsedMS <= points[0].TimeMillis {
		return float64(points[0].PositionPercent)
	}
	last := points[len(points)-1]
	if elapsedMS >= last.TimeMillis {
		return float64(last.PositionPercent)
	}
	for i := 0; i < len(points)-1; i++ {
		a := points[i]
		b := points[i+1]
		if elapsedMS < a.TimeMillis || elapsedMS > b.TimeMillis {
			continue
		}
		span := b.TimeMillis - a.TimeMillis
		if span <= 0 {
			return float64(b.PositionPercent)
		}
		t := float64(elapsedMS-a.TimeMillis) / float64(span)
		return float64(a.PositionPercent) + t*float64(b.PositionPercent-a.PositionPercent)
	}
	return float64(last.PositionPercent)
}
