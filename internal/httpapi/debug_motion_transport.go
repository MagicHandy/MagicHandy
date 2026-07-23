package httpapi

import (
	"context"

	diagnosticspkg "github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type deviceMotionDebugTransport struct {
	inner   transport.Transport
	server  *Server
	source  string
	verbose bool
}

func newDeviceMotionDebugTransport(inner transport.Transport, server *Server, source string, verbose bool) transport.Transport {
	if inner == nil {
		return nil
	}
	return &deviceMotionDebugTransport{
		inner:   inner,
		server:  server,
		source:  source,
		verbose: verbose,
	}
}

func (t *deviceMotionDebugTransport) Diagnostics() transport.TransportDiagnostics {
	return t.inner.Diagnostics()
}

func (t *deviceMotionDebugTransport) Stop(ctx context.Context, command transport.StopCommand) (transport.CommandResult, error) {
	result, err := t.inner.Stop(ctx, command)
	t.logCommand("hsp_stop", map[string]any{
		"reason": command.Reason,
	}, result, err)
	return result, err
}

func (t *deviceMotionDebugTransport) SetStrokeWindow(ctx context.Context, command transport.StrokeWindowCommand) (transport.CommandResult, error) {
	result, err := t.inner.SetStrokeWindow(ctx, command)
	if t.verbose {
		t.logCommand("stroke_window", map[string]any{
			"min_percent": command.MinPercent,
			"max_percent": command.MaxPercent,
		}, result, err)
	}
	return result, err
}

func (t *deviceMotionDebugTransport) AddHSP(ctx context.Context, command transport.HSPAddCommand) (transport.CommandResult, error) {
	result, err := t.inner.AddHSP(ctx, command)
	details := summarizeHSPSend(command.Points)
	details["stream_id"] = command.StreamID
	t.logCommand("hsp_add_sent", details, result, err)
	t.logSchedule("after_hsp_add")
	return result, err
}

func (t *deviceMotionDebugTransport) PlayHSP(ctx context.Context, command transport.HSPPlayCommand) (transport.CommandResult, error) {
	result, err := t.inner.PlayHSP(ctx, command)
	t.logCommand("hsp_play_sent", map[string]any{
		"stream_id":         command.StreamID,
		"start_time_ms":     command.StartTimeMillis,
		"server_time_ms":    command.ServerTimeMillis,
	}, result, err)
	t.logSchedule("after_hsp_play")
	return result, err
}

func (t *deviceMotionDebugTransport) logCommand(event string, details map[string]any, result transport.CommandResult, err error) {
	if t.server == nil || !t.server.handyMotionLogEnabled() {
		return
	}
	entry := diagnosticspkg.HandyLogEntry{
		PlaybackState: t.inner.Diagnostics().PlaybackState,
		Details:       details,
	}
	if err != nil {
		entry.Error = err.Error()
	} else if result.Status != "" && result.Status != "ok" && result.Status != "accepted" {
		entry.Error = result.Status
	}
	t.server.recordHandyMotionLog(event, t.source, entry)
}

func (t *deviceMotionDebugTransport) logSchedule(stage string) {
	if t.server == nil || !t.server.handyMotionLogEnabled() {
		return
	}
	metrics := t.server.outgoingSchedule().BufferMetrics()
	if !metrics.Active && !t.verbose {
		return
	}
	bufferAhead := metrics.BufferAheadMS
	streamElapsed := metrics.StreamElapsedMS
	position := metrics.PositionNow
	entry := diagnosticspkg.HandyLogEntry{
		PlaybackState: t.inner.Diagnostics().PlaybackState,
		BufferAheadMS: &bufferAhead,
		StreamElapsed: &streamElapsed,
		PositionPct:   &position,
		Details: map[string]any{
			"stage":        stage,
			"total_points": metrics.TotalPoints,
			"last_point_ms": metrics.LastPointMS,
			"stream_id":    metrics.StreamID,
			"device_idle_risk": metrics.BufferAheadMS < 650,
		},
	}
	event := "schedule_snapshot"
	if metrics.BufferAheadMS < 650 && metrics.Active {
		event = "device_starvation_risk"
		agentDebugLog("H1", "debug_motion_transport.go:logSchedule", "device_starvation_risk", map[string]any{
			"buffer_ahead_ms": metrics.BufferAheadMS,
			"stream_elapsed":  metrics.StreamElapsedMS,
			"last_point_ms":   metrics.LastPointMS,
			"playback_state":  t.inner.Diagnostics().PlaybackState,
			"stage":           stage,
		})
	}
	t.server.recordHandyMotionLog(event, t.source, entry)
}

func summarizeHSPSend(points []transport.TimedPoint) map[string]any {
	if len(points) == 0 {
		return map[string]any{"count": 0}
	}
	minPos, maxPos := points[0].PositionPercent, points[0].PositionPercent
	minT, maxT := points[0].TimeMillis, points[0].TimeMillis
	for _, point := range points {
		if point.PositionPercent < minPos {
			minPos = point.PositionPercent
		}
		if point.PositionPercent > maxPos {
			maxPos = point.PositionPercent
		}
		if point.TimeMillis < minT {
			minT = point.TimeMillis
		}
		if point.TimeMillis > maxT {
			maxT = point.TimeMillis
		}
	}
	return map[string]any{
		"count":    len(points),
		"time_min": minT,
		"time_max": maxT,
		"pos_min":  minPos,
		"pos_max":  maxPos,
	}
}
