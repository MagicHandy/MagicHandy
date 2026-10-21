package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

var (
	errVibratorHostOnly     = errors.New("only the host can raise the vibrator intensity or test it")
	errVibratorNotConnected = errors.New("connect the vibrator first")
	errVibratorNotSaved     = errors.New("add a vibrator connection before connecting it")
)

func (s *Server) vibratorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/transport/intiface/vibrator/select", s.handleVibratorSelect)
	mux.HandleFunc("POST /api/transport/intiface/vibrator/intensity", s.handleVibratorIntensity)
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
// vibrator stops first; the device is stopped before it is ready, and the
// live intensity starts again at zero.
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

// handleVibratorIntensity sets the live intensity. Lowering needs no
// controller admission, like Stop; raising needs the controlling host and
// the current Stop sequence, and is refused if the vibrator was stopped,
// reselected or reconnected since the request was prepared.
func (s *Server) handleVibratorIntensity(w http.ResponseWriter, r *http.Request) {
	vibrator, err := s.currentVibrator()
	if err != nil {
		finishUnreadBody(w, r)
		writeError(w, http.StatusConflict, err)
		return
	}
	generation := vibrator.ControlGeneration()
	var body struct {
		IntensityPercent int `json:"intensity_percent"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.IntensityPercent < 0 || body.IntensityPercent > 100 {
		writeError(w, http.StatusBadRequest, errors.New("the vibrator intensity must be between 0 and 100%"))
		return
	}
	target := float64(body.IntensityPercent) / 100
	if target > vibrator.Intensity() {
		if _, err := s.requestStopSequence(r); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		if !s.requireController(w, r) {
			return
		}
		if !s.capabilities(r).ConfigureHost {
			writeError(w, http.StatusForbidden, errVibratorHostOnly)
			return
		}
	}
	if _, err := vibrator.SetIntensityAt(generation, target); err != nil {
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

// vibratorConfig converts saved settings to the owner's fractions.
func vibratorConfig(settings config.VibrationSettings) transport.VibratorConfig {
	return transport.VibratorConfig{
		Profile:      vibration.Profile{Follow: settings.Follow, DepthEnd: settings.DepthEnd},
		Minimum:      float64(settings.MinimumPercent) / 100,
		Ceiling:      float64(settings.CeilingPercent) / 100,
		OffsetMillis: settings.TimingOffsetMillis,
	}
}

// applyVibratorSettingsTransition applies live vibration changes, and ends
// the vibrator's lease when it leaves the inventory.
func (s *Server) applyVibratorSettingsTransition(ctx context.Context, previous, next config.Settings) error {
	if previous.Device.HasDevice(config.DeviceVibrator) && !next.Device.HasDevice(config.DeviceVibrator) {
		return s.releaseIntifaceLease(ctx, config.DeviceVibrator)
	}
	if previous.Vibration != next.Vibration {
		s.intiface.mu.Lock()
		hub := s.intiface.hub
		s.intiface.mu.Unlock()
		if hub != nil {
			hub.Vibrator().Configure(vibratorConfig(next.Vibration))
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
