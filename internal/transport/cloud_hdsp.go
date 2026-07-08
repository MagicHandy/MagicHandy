package transport

import (
	"context"
	"errors"
	"math"
)

const (
	cloudPathHDSPXava = "hdsp/xava"
	hdspModeValue     = 2
	fullTravelMM      = 110.0
)

// DirectMoveCommand is a live position move for mouse/direct control.
type DirectMoveCommand struct {
	Normalized   float64
	DurationMS   int
	StrokeMinPct int
	StrokeMaxPct int
}

// DirectMotionCapable transports can stream live position moves (HDSP on cloud REST).
type DirectMotionCapable interface {
	DiagnosticsProvider
	PrepareDirectMotion(ctx context.Context, stroke StrokeWindowCommand) error
	EnqueueDirectMove(ctx context.Context, cmd DirectMoveCommand) error
	StopDirectMotion(ctx context.Context) error
}

type hdspMoveRequest struct {
	normalized float64
	durationMS int
	minPct     int
	maxPct     int
}

// PrepareDirectMotion enables HDSP mode and starts the move worker.
func (t *CloudRESTTransport) PrepareDirectMotion(ctx context.Context, stroke StrokeWindowCommand) error {
	if _, err := t.SetStrokeWindow(ctx, stroke); err != nil {
		return err
	}
	t.stopPlaybackForDirect(ctx)
	return t.ensureHDSP(ctx)
}

// EnqueueDirectMove queues a non-blocking HDSP move.
func (t *CloudRESTTransport) EnqueueDirectMove(ctx context.Context, cmd DirectMoveCommand) error {
	if err := t.ensureHDSP(ctx); err != nil {
		return err
	}
	t.hdspMu.Lock()
	ch := t.hdspMoves
	t.hdspMu.Unlock()
	if ch == nil {
		return errors.New("HDSP move worker is not running")
	}
	req := hdspMoveRequest{
		normalized: cmd.Normalized,
		durationMS: cmd.DurationMS,
		minPct:     cmd.StrokeMinPct,
		maxPct:     cmd.StrokeMaxPct,
	}
	select {
	case ch <- req:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- req:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// StopDirectMotion stops the HDSP worker.
func (t *CloudRESTTransport) StopDirectMotion(ctx context.Context) error {
	t.hdspMu.Lock()
	cancel := t.hdspCancel
	ch := t.hdspMoves
	t.hdspMoves = nil
	t.hdspCancel = nil
	t.hdspReady = false
	t.hdspMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if ch != nil {
		close(ch)
	}
	if t.hdspDone != nil {
		select {
		case <-t.hdspDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	t.stopPlaybackForDirect(ctx)
	return nil
}

func (t *CloudRESTTransport) stopPlaybackForDirect(ctx context.Context) {
	for _, spec := range []struct {
		path      string
		operation string
	}{
		{cloudPathHSSPStop, "hssp_stop"},
		{cloudPathHAMPStop, "hamp_stop"},
		{cloudPathHSPStop, "hsp_stop"},
	} {
		_, _ = t.dispatch(ctx, CloudRequest{
			Transport: cloudRESTName,
			Operation: spec.operation,
			Method:    "PUT",
			Path:      spec.path,
			Auth:      t.builder.auth,
		})
	}
}

func (t *CloudRESTTransport) ensureHDSP(ctx context.Context) error {
	t.hdspMu.Lock()
	if t.hdspReady && t.hdspMoves != nil {
		t.hdspMu.Unlock()
		return nil
	}
	t.hdspMu.Unlock()

	request := t.builder.BuildHDSPMode()
	if _, err := t.dispatch(ctx, request); err != nil {
		return err
	}

	t.hdspMu.Lock()
	defer t.hdspMu.Unlock()
	if t.hdspMoves == nil {
		t.hdspMoves = make(chan hdspMoveRequest, 64)
		workerCtx, cancel := context.WithCancel(context.Background())
		t.hdspCancel = cancel
		t.hdspDone = make(chan struct{})
		go t.hdspWorker(workerCtx)
	}
	t.hdspReady = true
	t.lastHDSPNorm = 0.5
	return nil
}

func (t *CloudRESTTransport) hdspWorker(ctx context.Context) {
	defer close(t.hdspDone)
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-t.hdspMoves:
			if !ok {
				return
			}
			req = t.coalesceHDSPMoves(req)
			_ = t.executeHDSPMove(ctx, req)
		}
	}
}

func (t *CloudRESTTransport) coalesceHDSPMoves(first hdspMoveRequest) hdspMoveRequest {
	t.hdspMu.Lock()
	ch := t.hdspMoves
	t.hdspMu.Unlock()
	if ch == nil {
		return first
	}
	latest := first
	for {
		select {
		case next := <-ch:
			latest = next
		default:
			return latest
		}
	}
}

func (t *CloudRESTTransport) executeHDSPMove(ctx context.Context, req hdspMoveRequest) error {
	clamped := clamp01(req.normalized)
	durationMS := req.durationMS
	if durationMS <= 0 {
		durationMS = 66
	}
	if durationMS > 10000 {
		durationMS = 10000
	}

	t.hdspMu.Lock()
	currentNorm := t.lastHDSPNorm
	t.hdspMu.Unlock()

	targetMM := pctNormToMM(clamped, req.minPct, req.maxPct)
	currentMM := pctNormToMM(currentNorm, req.minPct, req.maxPct)
	distanceMM := math.Abs(targetMM - currentMM)
	velocity := distanceMM / math.Max(0.05, float64(durationMS)/1000.0)
	if velocity < 5 {
		velocity = 5
	}
	if velocity > 400 {
		velocity = 400
	}

	request := t.builder.BuildHDSPMove(targetMM, int(math.Round(velocity)), durationMS >= 350)
	_, body, err := t.dispatchWithBody(ctx, request)
	if err == nil {
		t.hdspMu.Lock()
		t.lastHDSPNorm = clamped
		t.hdspMu.Unlock()
		// #region agent log
		agentDebugLog("H6", "cloud_hdsp.go:executeHDSPMove", "hdsp_execute_ok", map[string]any{
			"xa":           round2(targetMM),
			"va":           int(math.Round(velocity)),
			"normalized":   clamped,
			"response":     truncateDebugBody(body, 200),
		})
		// #endregion
	} else {
		// #region agent log
		agentDebugLog("H6", "cloud_hdsp.go:executeHDSPMove", "hdsp_execute_failed", map[string]any{
			"error":    err.Error(),
			"xa":       round2(targetMM),
			"va":       int(math.Round(velocity)),
			"response": truncateDebugBody(body, 200),
		})
		// #endregion
	}
	return err
}

func pctNormToMM(normalized float64, loPct, maxPct int) float64 {
	pct := float64(loPct) + (float64(maxPct)-float64(loPct))*clamp01(normalized)
	return fullTravelMM * (pct / 100.0)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// BuildHDSPMode shapes the Cloud REST HDSP mode request.
func (b *CloudRESTBuilder) BuildHDSPMode() CloudRequest {
	return CloudRequest{
		Transport: cloudRESTName,
		Operation: "hdsp_mode",
		Method:    "PUT",
		Path:      cloudPathMode2,
		Auth:      b.auth,
		Body:      map[string]int{"mode": hdspModeValue},
	}
}

// BuildHDSPMove shapes a Cloud REST v3 HDSP XAVA move request.
func (b *CloudRESTBuilder) BuildHDSPMove(positionMM float64, velocity int, stopOnTarget bool) CloudRequest {
	return CloudRequest{
		Transport: cloudRESTName,
		Operation: "hdsp_move",
		Method:    "PUT",
		Path:      cloudPathHDSPXava,
		Auth:      b.auth,
		Body: map[string]any{
			"xa":              round2(positionMM),
			"va":              velocity,
			"stop_on_target":  stopOnTarget,
			"immediate_rsp":   true,
		},
	}
}

// HDSPMovePath returns the REST path used for direct HDSP moves.
func HDSPMovePath() string { return cloudPathHDSPXava }
