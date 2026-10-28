package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

var (
	errVibratorHostOnly     = errors.New("only the host can resume or test the vibrator")
	errVibratorNotConnected = errors.New("connect the vibrator first")
	errVibratorNotSaved     = errors.New("add a vibrator connection before connecting it")
)

func (s *Server) vibratorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/transport/intiface/vibrator/select", s.handleVibratorSelect)
	mux.HandleFunc("POST /api/transport/intiface/vibrator/resume", s.handleVibratorResume)
	mux.HandleFunc("POST /api/transport/intiface/vibrator/test", s.handleVibratorTest)
}

// currentVibrator returns the vibrator of a connected hub it holds a lease on.
func (s *Server) currentVibrator() (*transport.IntifaceVibrator, error) {
	hub, err := s.currentIntifaceHub()
	if err != nil {
		return nil, errVibratorNotConnected
	}
	s.intiface.mu.Lock()
	held := s.intiface.leases[config.DeviceVibrator]
	s.intiface.mu.Unlock()
	if !held {
		return nil, errVibratorNotConnected
	}
	return hub.Vibrator(), nil
}

// vibratorMotionTransport admits an engine bound to the current vibrator.
func (s *Server) vibratorMotionTransport() (transport.Transport, error) {
	vibrator, err := s.currentVibrator()
	if err != nil {
		return nil, err
	}
	return vibrator.MotionTransport()
}

// handleVibratorSelect chooses the vibrator device. A run that includes the
// vibrator stops first, and the device is stopped before it is ready.
func (s *Server) handleVibratorSelect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		DeviceIndex uint32 `json:"device_index"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	vibrator, err := s.currentVibrator()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
	defer cancel()
	if settings, _ := s.store.Snapshot(); settings.Device.Plays(config.DeviceVibrator) {
		if s.modes != nil {
			s.modes.Stop("vibrator_selection_changed")
		}
		if err := s.stopAndClearMotionEngine(stopCtx, "vibrator_selection_changed"); err != nil {
			s.writeIntifaceResult(w, err)
			return
		}
	}
	err = vibrator.Select(stopCtx, body.DeviceIndex)
	if hub, hubErr := s.currentIntifaceHub(); err == nil && hubErr == nil && hub.Status().Scanning {
		// A finished search would otherwise keep the radio busy.
		scanCtx, cancelScan := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		if scanErr := hub.StopScanning(scanCtx); scanErr != nil {
			s.logger.Warn("stop Intiface scan after vibrator selection", "error", scanErr)
		}
		cancelScan()
	}
	s.writeIntifaceResult(w, err)
}

// handleVibratorResume lifts the suspension Emergency Stop left (Resume now).
// It plays nothing by itself. Like a test, it is an explicit host action fenced
// by the current Stop sequence and the vibrator's control generation.
func (s *Server) handleVibratorResume(w http.ResponseWriter, r *http.Request) {
	vibrator, err := s.currentVibrator()
	if err != nil {
		finishUnreadBody(w, r)
		writeError(w, http.StatusConflict, err)
		return
	}
	generation := vibrator.ControlGeneration()
	if !s.requireController(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errVibratorHostOnly)
		return
	}
	finishUnreadBody(w, r)
	if strings.TrimSpace(r.Header.Get(stopSequenceHeader)) == "" {
		writeError(w, http.StatusBadRequest, errors.New("resuming the vibrator needs the current Stop sequence"))
		return
	}
	if _, err := s.requestStopSequence(r); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	if err := vibrator.ResumeAt(generation); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientIntifaceSnapshot(r))
}

// handleVibratorTest plays a short, capped test while motion is idle. It is
// an explicit host action, fenced like a raise.
func (s *Server) handleVibratorTest(w http.ResponseWriter, r *http.Request) {
	vibrator, err := s.currentVibrator()
	if err != nil {
		finishUnreadBody(w, r)
		writeError(w, http.StatusConflict, err)
		return
	}
	generation := vibrator.ControlGeneration()
	if !s.requireController(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errVibratorHostOnly)
		return
	}
	finishUnreadBody(w, r)
	if strings.TrimSpace(r.Header.Get(stopSequenceHeader)) == "" {
		writeError(w, http.StatusBadRequest, errors.New("a vibrator test needs the current Stop sequence"))
		return
	}
	if _, err := s.requestStopSequence(r); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	if engine := s.currentMotionEngine(); engine != nil {
		if state := engine.Snapshot(); state.Running || state.Starting || state.Completing {
			writeError(w, http.StatusConflict, errors.New("pause playback before testing the vibrator"))
			return
		}
	}
	if err := vibrator.TestAt(generation); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientIntifaceSnapshot(r))
}

// stopForVibratorDisconnect ends a run that includes the vibrator, so an
// explicit disconnect never leaves the others running against a lost device.
func (s *Server) stopForVibratorDisconnect(ctx context.Context) error {
	settings, _ := s.store.Snapshot()
	if !settings.Device.Plays(config.DeviceVibrator) {
		return nil
	}
	if s.modes != nil {
		s.modes.Stop("vibrator_disconnected")
	}
	return s.stopAndClearMotionEngine(ctx, "vibrator_disconnected")
}

// vibratorConfig converts saved settings to the owner's fractions. Following
// speed spans the saved Handy speed band. Model control is effective only when
// the chat model has a vibration vocabulary; otherwise following runs at Max.
func vibratorConfig(settings config.Settings) transport.VibratorConfig {
	vibrationSettings := settings.Vibration
	motionSettings := settings.EffectiveMotion()
	low, high := motion.FollowSpeedBand(motionSettings)
	features := settings.LLM.ConversationSettings().ModelFeatures()
	return transport.VibratorConfig{
		Profile: vibration.Profile{Follow: vibrationSettings.Follow, DepthEnd: vibrationSettings.DepthEnd,
			SpeedLow: low, SpeedHigh: high},
		Minimum:         float64(vibrationSettings.MinimumPercent) / 100,
		Ceiling:         float64(vibrationSettings.CeilingPercent) / 100,
		OffsetMillis:    vibrationSettings.TimingOffsetMillis,
		ModelControl:    vibrationSettings.ModelSetsLevel() && (features.VibrationLevel || features.IndependentVibration),
		FollowOnly:      !vibrationSettings.ModelSetsLevel(),
		SpeedMinPercent: motionSettings.SpeedMinPercent,
		SpeedMaxPercent: motionSettings.SpeedMaxPercent,
	}
}

// applyVibratorSettingsTransition applies live vibration changes, and ends
// the vibrator's lease when it leaves the inventory.
func (s *Server) applyVibratorSettingsTransition(ctx context.Context, previous, next config.Settings) error {
	if previous.Device.HasDevice(config.DeviceVibrator) && !next.Device.HasDevice(config.DeviceVibrator) {
		return s.releaseIntifaceLease(ctx, config.DeviceVibrator)
	}
	if vibratorConfig(previous) != vibratorConfig(next) {
		s.intiface.mu.Lock()
		hub := s.intiface.hub
		s.intiface.mu.Unlock()
		if hub != nil {
			hub.Vibrator().Configure(vibratorConfig(next))
		}
	}
	return nil
}

// vibratorEngineSuffix names the vibrator binding an engine was built for.
func (s *Server) vibratorEngineSuffix() string {
	s.intiface.mu.Lock()
	defer s.intiface.mu.Unlock()
	if s.intiface.hub == nil {
		return "/vibrator-none"
	}
	return fmt.Sprintf("/vibrator-%d.%d", s.intiface.generation, s.intiface.hub.Vibrator().SelectionEpoch())
}
