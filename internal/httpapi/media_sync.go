package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/media"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	mediaHeartbeatTimeout      = 5 * time.Second
	mediaHeartbeatTick         = 500 * time.Millisecond
	mediaDriftLimitMillis      = int64(250)
	mediaDriftHardLimitMillis  = int64(750)
	mediaDriftConfirmations    = 2
	mediaClientRequestAgeLimit = 2 * time.Second
	maxMediaSessionFences      = 64
)

var errMediaMotionInterrupted = errors.New("media motion was interrupted")

type mediaSyncEvent struct {
	VideoID          string  `json:"video_id"`
	SessionID        string  `json:"session_id"`
	EventSequence    uint64  `json:"event_sequence"`
	State            string  `json:"state"`
	Event            string  `json:"event"`
	MediaTimeMillis  int64   `json:"media_time_ms"`
	ClientTimeMillis int64   `json:"client_time_ms"`
	PlaybackRate     float64 `json:"playback_rate"`
}

type mediaSessionFence struct {
	Sequence uint64
	Closed   bool
}

type mediaSyncStatus struct {
	Active           bool    `json:"active"`
	VideoID          string  `json:"video_id,omitempty"`
	State            string  `json:"state"`
	LastEvent        string  `json:"last_event,omitempty"`
	MediaTimeMillis  int64   `json:"media_time_ms,omitempty"`
	DriftMillis      int64   `json:"drift_ms,omitempty"`
	PlaybackRate     float64 `json:"playback_rate,omitempty"`
	MotionSpeedLimit int     `json:"motion_speed_limit_percent,omitempty"`
	RequiresReanchor bool    `json:"requires_reanchor,omitempty"`
	Message          string  `json:"message,omitempty"`
	UpdatedAt        string  `json:"updated_at,omitempty"`
}

func mediaMotionSpeedLimit(target motion.MotionTarget) int {
	if !target.MediaSpeedLimitEnabled {
		return 0
	}
	return target.SpeedPercent
}

type mediaHeartbeatTiming struct {
	driftMillis  int64
	breachCount  int
	calibrated   bool
	exceeded     bool
	requiresStop bool
}

// mediaSyncRuntime serializes one browser-clock session. It owns no device
// loop; every command still goes through Server's one shared motion.Engine.
type mediaSyncRuntime struct {
	server *Server
	now    func() time.Time

	lifecycleMu   sync.Mutex
	mu            sync.Mutex
	status        mediaSyncStatus
	script        *media.Funscript
	anchorMedia   int64
	anchorAt      time.Time
	lastSignal    time.Time
	heartbeatSeen bool
	driftBreaches int
	sessionID     string
	fences        map[string]mediaSessionFence
	fenceOrder    []string

	cancel context.CancelFunc
	done   chan struct{}
}

func newMediaSyncRuntime(server *Server) *mediaSyncRuntime {
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &mediaSyncRuntime{
		server: server,
		now:    func() time.Time { return time.Now().UTC() },
		status: mediaSyncStatus{State: "idle"},
		fences: make(map[string]mediaSessionFence),
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go runtime.watchHeartbeat(ctx)
	return runtime
}

func (m *mediaSyncRuntime) Status() mediaSyncStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *mediaSyncRuntime) Handle(ctx context.Context, event mediaSyncEvent, stopSequence uint64) (mediaSyncStatus, error) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if !m.acceptEvent(event) {
		return m.Status(), nil
	}
	if event.State == "playing" && event.Event != "heartbeat" {
		m.mu.Lock()
		if m.sessionID != event.SessionID {
			m.script = nil
		}
		m.sessionID = event.SessionID
		m.mu.Unlock()
	}

	switch event.State {
	case "playing":
		if event.Event == "heartbeat" {
			return m.handleHeartbeat(ctx, event)
		}
		return m.arm(ctx, event, stopSequence)
	case "seeking":
		return m.stopForEvent(ctx, event, "media_seeking", "seeking", false)
	case "paused":
		return m.stopForEvent(ctx, event, "media_paused", "paused", false)
	case "ended":
		return m.stopForEvent(ctx, event, "media_ended", "ended", true)
	case "closed":
		return m.stopForEvent(ctx, event, "media_player_closed", "idle", true)
	default:
		return m.Status(), errors.New("unsupported media sync state")
	}
}

func (m *mediaSyncRuntime) arm(ctx context.Context, event mediaSyncEvent, stopSequence uint64) (mediaSyncStatus, error) {
	if m.server.stopSequence.Load() != stopSequence {
		return m.Status(), errors.New("media playback was invalidated by Emergency Stop")
	}

	finishModeStop := func() {}
	if m.server.modes != nil {
		finishModeStop = m.server.modes.BeginUserStop()
	}
	defer finishModeStop()
	if _, err := m.server.stopActiveMotionForReplacement(ctx, "media_playback_replace"); err != nil {
		return m.setError(event, err), err
	}
	finishModeStop()
	// A cancelled mode may publish a late engine start while draining. Match
	// manual motion's second replacement check before arming the video.
	if _, err := m.server.stopActiveMotionForReplacement(ctx, "media_playback_replace"); err != nil {
		return m.setError(event, err), err
	}
	if m.server.stopSequence.Load() != stopSequence {
		return m.Status(), errors.New("media playback was invalidated by Emergency Stop")
	}
	if err := ctx.Err(); err != nil {
		return m.Status(), err
	}
	script, err := m.scriptFor(ctx, event.VideoID)
	if err != nil {
		return m.setError(event, err), err
	}
	timeline, err := script.TimelineFrom(event.MediaTimeMillis, event.PlaybackRate)
	if errors.Is(err, media.ErrFunscriptComplete) {
		status, stopErr := m.stopForEvent(ctx, event, "media_script_completed", "completed", false)
		if stopErr != nil {
			return status, stopErr
		}
		status.Message = "The paired script has ended; video playback can continue without motion."
		m.setStatus(status)
		return status, nil
	}
	if err != nil {
		return m.setError(event, err), err
	}
	if m.server.stopSequence.Load() != stopSequence {
		return m.Status(), errors.New("media playback was invalidated by Emergency Stop")
	}
	if err := ctx.Err(); err != nil {
		return m.Status(), err
	}

	engine, admission, err := m.server.motionEngineForStart()
	if err != nil {
		return m.setError(event, err), err
	}
	settings, _ := m.server.store.Snapshot()
	target := motion.MotionTarget{
		Label:   script.Name,
		Source:  motion.TargetSourceMedia,
		MediaID: timeline.ID,
		Media:   &timeline,
	}
	state, err := engine.StartAtGeneration(ctx, target, settings.Motion, admission)
	if err != nil {
		return m.setError(event, err), err
	}
	if err := ctx.Err(); err != nil {
		_, _ = engine.Stop(context.Background(), "media_start_cancelled")
		return m.Status(), err
	}
	if m.server.stopSequence.Load() != stopSequence {
		_, _ = engine.Stop(context.Background(), "media_start_invalidated")
		return m.Status(), errors.New("media playback was invalidated by Emergency Stop")
	}

	now := m.now()
	status := mediaSyncStatus{
		Active:           true,
		VideoID:          event.VideoID,
		State:            "following",
		LastEvent:        event.Event,
		MediaTimeMillis:  event.MediaTimeMillis,
		PlaybackRate:     event.PlaybackRate,
		MotionSpeedLimit: mediaMotionSpeedLimit(state.Target),
		Message:          "Device motion is synchronized to the paired script.",
		UpdatedAt:        now.Format(time.RFC3339Nano),
	}
	m.mu.Lock()
	m.anchorMedia = event.MediaTimeMillis
	m.anchorAt = now
	m.lastSignal = now
	m.status = status
	m.heartbeatSeen = false
	m.driftBreaches = 0
	m.mu.Unlock()
	m.trace("media_sync_armed", event, 0, false)
	return status, nil
}

func (m *mediaSyncRuntime) handleHeartbeat(ctx context.Context, event mediaSyncEvent) (mediaSyncStatus, error) {
	now := m.now()
	m.mu.Lock()
	status := m.status
	sessionID := m.sessionID
	m.mu.Unlock()
	if !status.Active || status.VideoID != event.VideoID || sessionID != event.SessionID {
		return status, errMediaMotionInterrupted
	}
	engine := m.server.currentMotionEngine()
	if engine == nil {
		status = m.interrupted(event, "Motion is no longer active; pause and play the video to re-arm it.")
		return status, errMediaMotionInterrupted
	}
	engineState := engine.Snapshot()
	if !engineState.Running || engineState.Target.Source != motion.TargetSourceMedia || engineState.Target.MediaID != event.VideoID {
		m.mu.Lock()
		activeScript := m.script
		m.mu.Unlock()
		if !engineState.Running && engineState.Target.Source == motion.TargetSourceMedia &&
			engineState.Target.MediaID == event.VideoID && activeScript != nil &&
			engineState.Phase >= 1 && engineState.LastError == "" {
			// The engine can finish a fraction before the browser reports the
			// final media millisecond because transport Play is acknowledged
			// before the held video resumes. A completed engine phase is the
			// authoritative signal; requiring exact browser time would turn that
			// harmless clock skew into a false competing-motion interruption.
			status = mediaSyncStatus{
				VideoID:          event.VideoID,
				State:            "completed",
				LastEvent:        event.Event,
				MediaTimeMillis:  event.MediaTimeMillis,
				PlaybackRate:     event.PlaybackRate,
				MotionSpeedLimit: mediaMotionSpeedLimit(engineState.Target),
				Message:          "The paired script has ended; video playback can continue without motion.",
				UpdatedAt:        now.Format(time.RFC3339Nano),
			}
			m.setStatus(status)
			m.trace("media_script_completed", event, 0, false)
			return status, nil
		}
		status = m.interrupted(event, "Another motion source took control; pause and play the video to re-arm it.")
		return status, errMediaMotionInterrupted
	}

	timing := m.observeHeartbeatTiming(event, now, status)
	if timing.calibrated {
		status.DriftMillis = 0
		status.MediaTimeMillis = event.MediaTimeMillis
		status.LastEvent = event.Event
		status.Message = "Device motion is synchronized to the paired script."
		status.UpdatedAt = now.Format(time.RFC3339Nano)
		m.mu.Lock()
		m.status = status
		m.mu.Unlock()
		return status, nil
	}

	drift := timing.driftMillis
	exceeded := timing.exceeded
	breaches := timing.breachCount
	if timing.requiresStop {
		if _, err := engine.Stop(ctx, "media_drift"); err != nil {
			return m.setError(event, err), err
		}
		status = mediaSyncStatus{
			VideoID:          event.VideoID,
			State:            "drifted",
			LastEvent:        event.Event,
			MediaTimeMillis:  event.MediaTimeMillis,
			DriftMillis:      drift,
			PlaybackRate:     event.PlaybackRate,
			MotionSpeedLimit: mediaMotionSpeedLimit(engineState.Target),
			RequiresReanchor: true,
			Message:          "Playback timing changed; motion stopped while the video re-arms.",
			UpdatedAt:        now.Format(time.RFC3339Nano),
		}
		m.setStatus(status)
		m.trace("media_sync_drift", event, drift, true)
		return status, nil
	}

	status.DriftMillis = drift
	status.MediaTimeMillis = event.MediaTimeMillis
	status.LastEvent = event.Event
	status.UpdatedAt = now.Format(time.RFC3339Nano)
	if exceeded {
		status.Message = "Device motion is synchronized; a timing variance is being confirmed."
		if breaches == 1 {
			m.trace("media_sync_drift_observed", event, drift, false)
		}
	} else {
		status.Message = "Device motion is synchronized to the paired script."
	}
	m.mu.Lock()
	m.status = status
	m.mu.Unlock()
	return status, nil
}

func (m *mediaSyncRuntime) observeHeartbeatTiming(
	event mediaSyncEvent,
	now time.Time,
	status mediaSyncStatus,
) mediaHeartbeatTiming {
	m.mu.Lock()
	anchorAt := m.anchorAt
	anchorMedia := m.anchorMedia
	heartbeatSeen := m.heartbeatSeen
	m.mu.Unlock()

	observedMedia := mediaTimeAtReceipt(event, now)
	expected := anchorMedia + int64(math.Round(float64(now.Sub(anchorAt).Milliseconds())*status.PlaybackRate))
	drift := observedMedia - expected
	rateChanged := math.Abs(event.PlaybackRate-status.PlaybackRate) > 0.001
	if !heartbeatSeen && !rateChanged && absInt64(drift) < mediaDriftHardLimitMillis {
		m.mu.Lock()
		m.anchorMedia = observedMedia
		m.anchorAt = now
		m.lastSignal = now
		m.heartbeatSeen = true
		m.driftBreaches = 0
		m.mu.Unlock()
		return mediaHeartbeatTiming{calibrated: true}
	}

	exceeded := absInt64(drift) > mediaDriftLimitMillis
	m.mu.Lock()
	if exceeded {
		m.driftBreaches++
	} else {
		m.driftBreaches = 0
	}
	breaches := m.driftBreaches
	m.lastSignal = now
	m.mu.Unlock()
	return mediaHeartbeatTiming{
		driftMillis: drift,
		breachCount: breaches,
		exceeded:    exceeded,
		requiresStop: rateChanged || absInt64(drift) >= mediaDriftHardLimitMillis ||
			breaches >= mediaDriftConfirmations,
	}
}

func (m *mediaSyncRuntime) stopForEvent(ctx context.Context, event mediaSyncEvent, reason, state string, releaseScript bool) (mediaSyncStatus, error) {
	m.mu.Lock()
	current := m.status
	currentSession := m.sessionID
	m.mu.Unlock()
	if currentSession != "" && currentSession != event.SessionID {
		return current, nil
	}
	if current.VideoID != "" && event.VideoID != "" && current.VideoID != event.VideoID {
		return current, nil
	}
	engine := m.server.currentMotionEngine()
	if engine != nil {
		engineState := engine.Snapshot()
		if engineState.Target.Source == motion.TargetSourceMedia && (engineState.Running || engineState.Paused || engineState.Completing) {
			if _, err := engine.Stop(ctx, reason); err != nil {
				return m.setError(event, err), err
			}
		}
	}
	now := m.now()
	status := mediaSyncStatus{
		VideoID:         event.VideoID,
		State:           state,
		LastEvent:       event.Event,
		MediaTimeMillis: event.MediaTimeMillis,
		PlaybackRate:    event.PlaybackRate,
		Message:         mediaSyncMessage(state),
		UpdatedAt:       now.Format(time.RFC3339Nano),
	}
	m.mu.Lock()
	m.anchorAt = time.Time{}
	m.anchorMedia = 0
	m.lastSignal = time.Time{}
	m.heartbeatSeen = false
	m.driftBreaches = 0
	if releaseScript {
		m.script = nil
	}
	m.status = status
	m.mu.Unlock()
	m.trace(reason, event, 0, false)
	return status, nil
}

// acceptEvent makes close and Stop-adjacent lifecycle events monotonic per
// mounted player. A delayed request from an unmounted player can therefore
// neither restart its closed session nor stop a newer session for the same
// video.
func (m *mediaSyncRuntime) acceptEvent(event mediaSyncEvent) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	fence, exists := m.fences[event.SessionID]
	if exists && event.EventSequence <= fence.Sequence {
		return false
	}
	if !exists {
		m.fenceOrder = append(m.fenceOrder, event.SessionID)
	}
	closed := fence.Closed
	fence.Sequence = event.EventSequence
	if event.State == "closed" {
		fence.Closed = true
	}
	m.fences[event.SessionID] = fence
	m.trimFencesLocked()
	return !closed
}

func (m *mediaSyncRuntime) trimFencesLocked() {
	for len(m.fences) > maxMediaSessionFences {
		removed := false
		for index, sessionID := range m.fenceOrder {
			if sessionID == m.sessionID {
				continue
			}
			delete(m.fences, sessionID)
			m.fenceOrder = append(m.fenceOrder[:index], m.fenceOrder[index+1:]...)
			removed = true
			break
		}
		if !removed {
			return
		}
	}
}

func (m *mediaSyncRuntime) scriptFor(ctx context.Context, videoID string) (media.Funscript, error) {
	m.mu.Lock()
	if m.script != nil && m.script.VideoID == videoID {
		script := *m.script
		m.mu.Unlock()
		return script, nil
	}
	m.mu.Unlock()
	script, err := m.server.media.LoadFunscript(ctx, videoID)
	if err != nil {
		return media.Funscript{}, err
	}
	m.mu.Lock()
	m.script = &script
	m.mu.Unlock()
	return script, nil
}

func (m *mediaSyncRuntime) interrupted(event mediaSyncEvent, message string) mediaSyncStatus {
	status := mediaSyncStatus{
		VideoID:         event.VideoID,
		State:           "interrupted",
		LastEvent:       event.Event,
		MediaTimeMillis: event.MediaTimeMillis,
		PlaybackRate:    event.PlaybackRate,
		Message:         message,
		UpdatedAt:       m.now().Format(time.RFC3339Nano),
	}
	m.setStatus(status)
	return status
}

func (m *mediaSyncRuntime) setError(event mediaSyncEvent, err error) mediaSyncStatus {
	message := "Paired-script motion could not be synchronized."
	switch {
	case errors.Is(err, media.ErrVideoNotFound), errors.Is(err, media.ErrFunscriptNotFound),
		errors.Is(err, media.ErrFunscriptUnavailable):
		message = "Paired funscript is unavailable."
	case errors.Is(err, media.ErrFunscriptInvalid), errors.Is(err, media.ErrFunscriptTooLarge):
		message = err.Error()
	case errors.Is(err, context.Canceled):
		message = "The previous synchronization request was replaced."
	default:
		if safeMessage := m.server.safeMotionErrorMessage(err); safeMessage != "" {
			message = safeMessage
		}
	}
	status := mediaSyncStatus{
		VideoID:         event.VideoID,
		State:           "error",
		LastEvent:       event.Event,
		MediaTimeMillis: event.MediaTimeMillis,
		PlaybackRate:    event.PlaybackRate,
		Message:         message,
		UpdatedAt:       m.now().Format(time.RFC3339Nano),
	}
	m.setStatus(status)
	return status
}

func (m *mediaSyncRuntime) setStatus(status mediaSyncStatus) {
	m.mu.Lock()
	m.status = status
	if !status.Active {
		m.anchorAt = time.Time{}
		m.anchorMedia = 0
		m.lastSignal = time.Time{}
		m.heartbeatSeen = false
		m.driftBreaches = 0
	}
	m.mu.Unlock()
}

func (m *mediaSyncRuntime) Invalidate(reason string) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	status := m.Status()
	status.Active = false
	status.State = "stopped"
	status.LastEvent = reason
	status.RequiresReanchor = false
	status.Message = "Motion was stopped. Press play to start a new synchronized run."
	status.UpdatedAt = m.now().Format(time.RFC3339Nano)
	m.setStatus(status)
}

func (m *mediaSyncRuntime) watchHeartbeat(ctx context.Context) {
	defer close(m.done)
	ticker := time.NewTicker(mediaHeartbeatTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.expireHeartbeat(now.UTC())
		}
	}
}

func (m *mediaSyncRuntime) expireHeartbeat(now time.Time) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.mu.Lock()
	active := m.status.Active
	lastSignal := m.lastSignal
	videoID := m.status.VideoID
	rate := m.status.PlaybackRate
	sessionID := m.sessionID
	m.mu.Unlock()
	if !active || lastSignal.IsZero() || now.Sub(lastSignal) <= mediaHeartbeatTimeout {
		return
	}
	event := mediaSyncEvent{VideoID: videoID, SessionID: sessionID, State: "paused", Event: "heartbeat_timeout", PlaybackRate: rate}
	_, _ = m.stopForEvent(context.Background(), event, "media_heartbeat_lost", "timed_out", false)
}

func (m *mediaSyncRuntime) Shutdown() {
	if m == nil || m.cancel == nil {
		return
	}
	m.cancel()
	<-m.done
	m.cancel = nil
}

func (m *mediaSyncRuntime) trace(reason string, event mediaSyncEvent, drift int64, reanchor bool) {
	if m.server.traces == nil {
		return
	}
	m.server.traces.Add(diagnostics.MotionTraceRow{
		Source: motion.TargetSourceMedia,
		Reason: reason,
		Target: &diagnostics.MotionTraceTarget{
			Label:           "Video funscript",
			MediaIdentifier: event.VideoID,
		},
		Annotation: fmt.Sprintf(
			"media_ms=%d playback_rate=%.3f drift_ms=%d reanchor=%t",
			event.MediaTimeMillis,
			event.PlaybackRate,
			drift,
			reanchor,
		),
	})
}

func mediaSyncMessage(state string) string {
	switch state {
	case "paused":
		return "Video paused; device motion stopped."
	case "seeking":
		return "Seeking; queued device motion was cleared."
	case "ended", "completed":
		return "Paired-script motion finished."
	case "timed_out":
		return "Video heartbeat was lost; device motion stopped."
	default:
		return "Paired-script motion is idle."
	}
}

func mediaTimeAtReceipt(event mediaSyncEvent, now time.Time) int64 {
	observed := event.MediaTimeMillis
	if event.ClientTimeMillis <= 0 || event.PlaybackRate <= 0 || math.IsNaN(event.PlaybackRate) || math.IsInf(event.PlaybackRate, 0) {
		return observed
	}
	requestAgeMillis := now.UnixMilli() - event.ClientTimeMillis
	if requestAgeMillis < 0 || requestAgeMillis > mediaClientRequestAgeLimit.Milliseconds() {
		return observed
	}
	return observed + int64(math.Round(float64(requestAgeMillis)*event.PlaybackRate))
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
