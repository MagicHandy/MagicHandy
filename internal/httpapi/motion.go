package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

var errMotionUnavailable = errors.New("motion engine is unavailable for the configured transport")

// motionRuntime owns the live motion engine used by the manual UI controls.
// It is nil-safe: when the configured transport is not a full command
// transport, the engine is absent and motion endpoints report unavailable
// rather than panicking.
type motionRuntime struct {
	mu        sync.Mutex
	engine    *motion.Engine
	owner     string
	transport transport.Transport
}

func newMotionRuntime(runtime Runtime) motionRuntime {
	return motionRuntime{transport: runtime.MotionTransport}
}

// motionRequest is the optional body for start/target control.
type motionRequest struct {
	Pattern      string `json:"pattern,omitempty"`
	SpeedPercent int    `json:"speed_percent,omitempty"`
}

func (r motionRequest) target() motion.MotionTarget {
	return motion.MotionTarget{
		Label:        "Manual",
		Source:       "manual_ui",
		PatternID:    motion.PatternID(r.Pattern),
		SpeedPercent: r.SpeedPercent,
	}
}

// motionState returns a UI-facing snapshot; the "available" flag lets the
// frontend show an honest "motion unavailable" state instead of guessing.
func (s *Server) motionState() any {
	if engine := s.currentMotionEngine(); engine != nil {
		snapshot := engine.Snapshot()
		if snapshot.Running || snapshot.Paused {
			return map[string]any{
				"available": true,
				"engine":    snapshot,
			}
		}
	}
	if _, err := s.newSelectedMotionTransport(); err != nil {
		return map[string]any{
			"available": false,
			"error":     err.Error(),
		}
	}
	return map[string]any{
		"available": true,
	}
}

func (s *Server) handleMotionState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.motionState())
}

func (s *Server) handleMotionEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming responses are unavailable"))
		return
	}

	clientID := clientIDFromRequest(r)
	setSSEHeaders(w)
	w.WriteHeader(http.StatusOK)

	emit := func() bool {
		if clientID != "" {
			s.controller.Touch(clientID)
		}
		if err := writeSSE(w, "motion", s.motionState()); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !emit() {
		return
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !emit() {
				return
			}
		}
	}
}

func (s *Server) handleMotionStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}

	var body motionRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	engine, err := s.motionEngineForStart()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	settings, _ := s.store.Snapshot()
	state, err := engine.Start(r.Context(), body.target(), settings.Motion)
	s.writeMotionResult(w, state, err)
}

func (s *Server) handleMotionTarget(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}

	engine := s.currentMotionEngine()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, errMotionUnavailable)
		return
	}
	var body motionRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	state, err := engine.ApplyTarget(r.Context(), body.target(), "ui_target")
	s.writeMotionResult(w, state, err)
}

// handleMotionQuick patches motion settings (speed/stroke/direction), persists
// them, and applies them to any active loop immediately, so quick controls take
// effect without a save-and-restart cycle.
func (s *Server) handleMotionQuick(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}

	var body struct {
		SpeedMinPercent  *int    `json:"speed_min_percent,omitempty"`
		SpeedMaxPercent  *int    `json:"speed_max_percent,omitempty"`
		StrokeMinPercent *int    `json:"stroke_min_percent,omitempty"`
		StrokeMaxPercent *int    `json:"stroke_max_percent,omitempty"`
		ReverseDirection *bool   `json:"reverse_direction,omitempty"`
		Style            *string `json:"style,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	current, _ := s.store.Snapshot()
	motionSettings := current.Motion
	if body.SpeedMinPercent != nil {
		motionSettings.SpeedMinPercent = *body.SpeedMinPercent
	}
	if body.SpeedMaxPercent != nil {
		motionSettings.SpeedMaxPercent = *body.SpeedMaxPercent
	}
	if body.StrokeMinPercent != nil {
		motionSettings.StrokeMinPercent = *body.StrokeMinPercent
	}
	if body.StrokeMaxPercent != nil {
		motionSettings.StrokeMaxPercent = *body.StrokeMaxPercent
	}
	if body.ReverseDirection != nil {
		motionSettings.ReverseDirection = *body.ReverseDirection
	}
	if body.Style != nil {
		motionSettings.Style = *body.Style
	}
	current.Motion = motionSettings

	saved, err := s.store.Save(current)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.refreshActiveMotion(r.Context(), saved.Motion)

	payload := map[string]any{"motion": saved.Public().Motion}
	if engine := s.currentMotionEngine(); engine != nil {
		payload["engine"] = engine.Snapshot()
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleMotionStop(w http.ResponseWriter, r *http.Request) {
	// An explicit user stop always ends autonomous modes first, so no
	// keepalive or planner can restart motion the user just stopped.
	if s.modes != nil {
		s.modes.NotifyUserStop()
	}
	s.cancelFreestyleChaosMotion(r.Context())
	s.cancelChatChaosMotion(r.Context())
	engine := s.currentMotionEngine()
	if engine == nil {
		writeJSON(w, http.StatusOK, s.motionState())
		return
	}
	state, err := engine.Stop(r.Context(), "ui_stop")
	s.writeMotionResult(w, state, err)
}

// handleMotionPause freezes active motion (phase retained for resume). Unlike
// Stop it is a control action, so read-only clients cannot trigger it.
func (s *Server) handleMotionPause(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	engine := s.currentMotionEngine()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, errMotionUnavailable)
		return
	}
	state, err := engine.Pause(r.Context(), "ui_pause")
	s.writeMotionResult(w, state, err)
}

func (s *Server) handleMotionResume(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	engine := s.currentMotionEngine()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, errMotionUnavailable)
		return
	}
	state, err := engine.Resume(r.Context(), "ui_resume")
	s.writeMotionResult(w, state, err)
}

// writeMotionResult always returns the resolved engine state so the UI can
// reconcile optimistic controls, and reports transport failures as 502 with the
// state attached rather than a bare error.
func (s *Server) writeMotionResult(w http.ResponseWriter, state motion.ActiveMotionState, err error) {
	status := http.StatusOK
	payload := map[string]any{"available": true, "engine": state}
	if err != nil {
		status = http.StatusBadGateway
		payload["error"] = err.Error()
	}
	writeJSON(w, status, payload)
}

// refreshActiveMotion applies saved settings to running motion immediately so
// quick controls take effect without a stop/start (ADR 0002, Invariant 9).
func (s *Server) refreshActiveMotion(ctx context.Context, settings config.MotionSettings) {
	engine := s.currentMotionEngine()
	if engine == nil {
		return
	}
	if !engine.Snapshot().Running {
		return
	}
	_, _ = engine.RefreshSettings(ctx, settings, "settings_saved")
}

func (s *Server) applySettingsRuntimeTransition(ctx context.Context, previous config.Settings, next config.Settings) {
	if previous.Device.HSPDispatchOwner != next.Device.HSPDispatchOwner {
		// Owner switches stop first — including any autonomous mode.
		if s.modes != nil {
			s.modes.Stop("dispatch_owner_changed")
		}
		s.stopAndClearMotionEngine(ctx, "dispatch_owner_changed")
		s.resetCloudTransport()
		return
	}
	if previous.Device.HandyConnectionKey != next.Device.HandyConnectionKey {
		s.resetCloudTransport()
	}
	s.refreshActiveMotion(ctx, next.Motion)
}

func (s *Server) stopAndClearMotionEngine(ctx context.Context, reason string) {
	s.motion.mu.Lock()
	engine := s.motion.engine
	s.motion.engine = nil
	s.motion.owner = ""
	s.motion.mu.Unlock()

	if engine == nil {
		return
	}
	if engine.Snapshot().Running {
		_, _ = engine.Stop(ctx, reason)
	}
}

// Close stops any active motion loop so no goroutine keeps commanding the
// device after shutdown (goroutine-lifecycle safety gate).
func (s *Server) Close() {
	s.closeLLM()
	if s.modes != nil {
		s.modes.Shutdown()
	}
	s.stopAndClearMotionEngine(context.Background(), "server_shutdown")
	s.stopManualQueuePlayer(context.Background())
	s.cancelChatChaosMotion(context.Background())
	s.personalization.Close()
	if s.library != nil && s.library.Store() != nil {
		_ = s.library.Store().Close()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
}

func (s *Server) motionEngineForStart() (*motion.Engine, error) {
	settings, _ := s.store.Snapshot()
	owner := settings.Device.HSPDispatchOwner
	engine, engineOwner := s.currentMotionEngineWithOwner()
	if engine != nil && engineOwner != owner {
		s.stopAndClearMotionEngine(context.Background(), "dispatch_owner_changed")
	} else if engine != nil && engine.Snapshot().Running {
		return engine, nil
	}
	commandTransport, err := s.newSelectedMotionTransport()
	if err != nil {
		return nil, err
	}
	engine, err = motion.NewEngine(motion.EngineOptions{
		Transport: commandTransport,
		Traces:    s.traces,
	})
	if err != nil {
		return nil, err
	}
	s.setMotionEngine(engine, owner)
	return engine, nil
}

func (s *Server) newSelectedMotionTransport() (transport.Transport, error) {
	var inner transport.Transport
	if s.motion.transport != nil {
		inner = s.motion.transport
	} else {
		settings, _ := s.store.Snapshot()
		var err error
		switch settings.Device.HSPDispatchOwner {
		case config.DispatchOwnerCloudREST:
			inner, err = s.newCloudTransport()
		case config.DispatchOwnerBrowserBluetooth:
			snapshot := s.bluetooth.bridge.Snapshot()
			if !snapshot.Ready {
				message := snapshot.Message
				if message == "" {
					message = "Bluetooth is not connected."
				}
				return nil, fmt.Errorf("browser Bluetooth is not ready: %s", message)
			}
			inner, err = s.newBluetoothTransport()
		case config.DispatchOwnerIntiface:
			inner, err = s.newIntifaceTransport()
		default:
			return nil, errMotionUnavailable
		}
		if err != nil {
			return nil, err
		}
	}
	return transport.NewRecordingTransport(inner, s.outgoingSchedule()), nil
}

func (s *Server) wrapMotionCommandTransport(recording transport.Transport, source string) transport.Transport {
	settings, _ := s.store.Snapshot()
	if !settings.Diagnostics.ShouldLogHandyMotion() {
		return recording
	}
	return newDeviceMotionDebugTransport(recording, s, source, settings.Diagnostics.VerboseHandyMotion())
}

// newMotionCommandTransport drives procedural/manual-queue players. When the
// configured device is not connected it falls back to an in-memory transport so
// the live visualizer still mirrors outgoing HSP.
func (s *Server) newMotionCommandTransport() (transport.Transport, error) {
	settings, _ := s.store.Snapshot()
	if !s.deviceMotionReady(settings) {
		if s.motion.transport != nil {
			return s.wrapMotionCommandTransport(
				transport.NewRecordingTransport(s.motion.transport, s.outgoingSchedule()),
				"chat_auto",
			), nil
		}
		selected, err := s.newSelectedMotionTransport()
		if err == nil {
			return s.wrapMotionCommandTransport(selected, "chat_auto"), nil
		}
		return s.wrapMotionCommandTransport(
			transport.NewRecordingTransport(transport.NewFake(), s.outgoingSchedule()),
			"chat_auto",
		), nil
	}
	selected, err := s.newSelectedMotionTransport()
	if err != nil {
		return nil, err
	}
	return s.wrapMotionCommandTransport(selected, "chat_auto"), nil
}

func (s *Server) deviceMotionReady(settings config.Settings) bool {
	intiface := s.intifaceDiagnostics()
	cloud := s.cloudDiagnostics()
	state := resolveLsoDeviceStatus(settings, intiface, cloud, s.cloud.baseURL)
	return state.Connected
}

func (s *Server) activeProceduralPlayerSnapshot() (manualqueue.Snapshot, bool) {
	s.chatAuto.mu.Lock()
	autoPlayer := s.chatAuto.player
	s.chatAuto.mu.Unlock()
	if snap, ok := snapshotIfRunning(autoPlayer); ok {
		return snap, true
	}

	s.chatChaos.mu.Lock()
	chatPlayer := s.chatChaos.player
	s.chatChaos.mu.Unlock()
	if snap, ok := snapshotIfRunning(chatPlayer); ok {
		return snap, true
	}

	s.freestyleChaos.mu.Lock()
	freestylePlayer := s.freestyleChaos.player
	s.freestyleChaos.mu.Unlock()
	if snap, ok := snapshotIfRunning(freestylePlayer); ok {
		return snap, true
	}

	s.manualQueue.mu.Lock()
	manualPlayer := s.manualQueue.player
	s.manualQueue.mu.Unlock()
	if snap, ok := snapshotIfRunning(manualPlayer); ok {
		return snap, true
	}
	return manualqueue.Snapshot{}, false
}

func snapshotIfRunning(player *manualqueue.Player) (manualqueue.Snapshot, bool) {
	if player == nil {
		return manualqueue.Snapshot{}, false
	}
	snap := player.Snapshot()
	if snap.Running && !snap.Paused {
		return snap, true
	}
	return manualqueue.Snapshot{}, false
}

func (s *Server) chatChaosActive() bool {
	s.chatChaos.mu.Lock()
	player := s.chatChaos.player
	s.chatChaos.mu.Unlock()
	if player == nil {
		return false
	}
	return player.Snapshot().Running
}

func (s *Server) currentMotionEngine() *motion.Engine {
	s.motion.mu.Lock()
	defer s.motion.mu.Unlock()
	return s.motion.engine
}

func (s *Server) currentMotionEngineWithOwner() (*motion.Engine, string) {
	s.motion.mu.Lock()
	defer s.motion.mu.Unlock()
	return s.motion.engine, s.motion.owner
}

func (s *Server) setMotionEngine(engine *motion.Engine, owner string) {
	s.motion.mu.Lock()
	defer s.motion.mu.Unlock()
	s.motion.engine = engine
	s.motion.owner = owner
}
