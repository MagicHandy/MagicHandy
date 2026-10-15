package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/workerclient"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/estim"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	coyoteConnectTimeout = 45 * time.Second
	maxCoyoteStep        = 10
)

var (
	errCoyoteOutputNotSelected = errors.New("add a Coyote connection before connecting it")
	errCoyoteSafetyPending     = errors.New("read and accept the Coyote safety notes before connecting it")
	errCoyoteHostOnly          = errors.New("only the host can connect the Coyote or raise its strength")
	errCoyoteBadStep           = errors.New("strength steps must name channel a or b and change by 1 to 10")
)

type coyoteRuntime struct {
	owner       *transport.Coyote
	helperPath  string
	discoverer  ble.Discoverer
	discoveryMu sync.Mutex
	discovering bool
	candidates  map[string]coyoteCandidate
}

type coyoteSnapshot struct {
	Output    string                 `json:"output"`
	Available bool                   `json:"available"`
	Status    transport.CoyoteStatus `json:"status"`
}

type coyoteStrengthRequest struct {
	Channel string `json:"channel"`
	Delta   int    `json:"delta"`
}

// configureCoyote builds the owner once. Simulation uses the in-process
// device; otherwise the Bluetooth worker beside the executable (or in the
// data directory's tools folder) owns the radio.
func (s *Server) configureCoyote(runtime Runtime, settings config.Settings, logger *slog.Logger) {
	connector := runtime.CoyoteConnector
	if connector == nil {
		s.coyote.helperPath = resolveWorkerBinary("", runtime.ExecutablePath, s.store.DataDir(), bluetoothWorkerName)
		connector = workerclient.NewConnector(workerclient.Options{Profile: ble.ProfileCoyote, Path: s.coyote.helperPath, Logger: logger})
	}
	s.coyote.owner = transport.NewCoyote(transport.CoyoteOptions{Connector: connector})
	s.coyote.discoverer, _ = connector.(ble.Discoverer)
	_ = s.coyote.owner.Configure(coyoteConfig(settings.Estim))
}

func coyoteConfig(settings config.EstimSettings) transport.CoyoteConfig {
	return transport.CoyoteConfig{
		Limits: estim.Limits{
			StrengthA:         settings.MaxStrengthA,
			StrengthB:         settings.MaxStrengthB,
			FrequencyBalanceA: settings.FrequencyBalanceA,
			FrequencyBalanceB: settings.FrequencyBalanceB,
			IntensityBalanceA: settings.IntensityBalanceA,
			IntensityBalanceB: settings.IntensityBalanceB,
		},
		Profile: estim.Profile{
			Follow:       settings.Follow,
			MinHz:        settings.FrequencyMinHz,
			MaxHz:        settings.FrequencyMaxHz,
			Balance:      settings.Balance,
			SwapChannels: settings.SwapChannels,
		},
		OffsetMillis:      settings.TimingOffsetMillis,
		RememberedAddress: settings.RememberedAddress,
	}
}

func (s *Server) coyoteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/transport/coyote/status", s.handleCoyoteStatus)
	mux.HandleFunc("POST /api/transport/coyote/connect", s.handleCoyoteConnect)
	mux.HandleFunc("POST /api/transport/coyote/discover", s.handleCoyoteDiscover)
	mux.HandleFunc("POST /api/transport/coyote/disconnect", s.handleCoyoteDisconnect)
	mux.HandleFunc("POST /api/transport/coyote/strength", s.handleCoyoteStrength)
	mux.HandleFunc("POST /api/transport/coyote/test", s.handleCoyoteTest)
	mux.HandleFunc("POST /api/transport/coyote/zero", s.handleCoyoteZero)
	mux.HandleFunc("PUT /api/settings/device/output", s.handlePutDeviceOutput)
	mux.HandleFunc("PUT /api/settings/device/connections", s.handlePutDeviceConnection)
	mux.HandleFunc("PUT /api/settings/estim/safety", s.handlePutEstimSafety)
}

// handlePutDeviceOutput switches what the shared frame drives. It applies
// immediately: running motion stops, and a Coyote that leaves the output is
// zeroed and disconnected.
func (s *Server) handlePutDeviceOutput(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var update struct {
		Output string `json:"output"`
	}
	if err := decodeJSON(r, &update); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	output := strings.TrimSpace(update.Output)
	if output != config.DeviceOutputNone && output != config.DeviceOutputHandy && output != config.DeviceOutputCoyote && output != config.DeviceOutputHandyCoyote {
		writeError(w, http.StatusBadRequest, errors.New("unknown device output"))
		return
	}
	s.writeImmediateSettings(w, r, func(current config.Settings) (config.Settings, error) {
		current.Device = current.Device.WithPlaybackOutput(output)
		return current, nil
	})
}

// handlePutEstimSafety records that the host read the Coyote safety notes.
// Withdrawing it disconnects nothing by itself; it blocks the next connect.
func (s *Server) handlePutEstimSafety(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var update struct {
		Acknowledged bool `json:"acknowledged"`
	}
	if err := decodeJSON(r, &update); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.writeImmediateSettings(w, r, func(current config.Settings) (config.Settings, error) {
		current.Estim.SafetyAcknowledged = update.Acknowledged
		return current, nil
	})
}

func (s *Server) writeImmediateSettings(w http.ResponseWriter, r *http.Request, mutate func(config.Settings) (config.Settings, error)) {
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), mutate)
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, saveErr)
		return
	}
	payload := map[string]any{"settings": s.clientSettings(r, saved.Public())}
	status := http.StatusOK
	if runtimeErr != nil {
		status = http.StatusBadGateway
		payload["error"] = "the setting was saved, but the device runtime could not be updated"
	}
	writeJSON(w, status, payload)
}

func (s *Server) handleCoyoteStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.clientCoyoteSnapshot(r))
}

func (s *Server) handleCoyoteConnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errCoyoteHostOnly)
		return
	}
	var request struct {
		CandidateID string `json:"candidate_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.coyote.discoveryMu.Lock()
	defer s.coyote.discoveryMu.Unlock()
	address, err := s.coyote.takeCandidate(request.CandidateID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	s.settingsLifecycleMu.Lock()
	defer s.settingsLifecycleMu.Unlock()
	settings, _ := s.store.Snapshot()
	switch {
	case !settings.Device.HasDevice(config.DeviceOutputCoyote):
		writeError(w, http.StatusConflict, errCoyoteOutputNotSelected)
		return
	case !settings.Estim.SafetyAcknowledged:
		writeError(w, http.StatusConflict, errCoyoteSafetyPending)
		return
	}
	if err := s.coyote.owner.Configure(coyoteConfig(settings.Estim)); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), coyoteConnectTimeout)
	defer cancel()
	var status transport.CoyoteStatus
	if address != "" {
		status, err = s.coyote.owner.ConnectAddress(ctx, address)
	} else {
		status, err = s.coyote.owner.Connect(ctx)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	s.rememberCoyote(ctx, status)
	writeJSON(w, http.StatusOK, s.clientCoyoteSnapshot(r))
}

// rememberCoyote stores the connected device's address so the next connect
// skips pairing mode on recent firmware. The simulator is never remembered.
func (s *Server) rememberCoyote(ctx context.Context, status transport.CoyoteStatus) {
	address := strings.TrimSpace(status.Address)
	if status.Simulated || address == "" {
		return
	}
	_, _, err := s.store.UpdateContext(ctx, func(settings config.Settings) (config.Settings, error) {
		settings.Estim.RememberedAddress = address
		return settings, nil
	})
	if err != nil {
		s.logger.Warn("could not remember the Coyote address", "error", err)
	}
}

func (s *Server) handleCoyoteDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	settings, _ := s.store.Snapshot()
	if config.UsesCoyote(settings.Device.Output) {
		// Explicitly disconnecting a participant ends this run, including a linked run.
		if s.modes != nil {
			s.modes.Stop("coyote_disconnected")
		}
		if err := s.stopAndClearMotionEngine(r.Context(), "coyote_disconnected"); err != nil {
			_, disconnectErr := s.coyote.owner.Disconnect()
			writeError(w, http.StatusBadGateway, errors.Join(err, disconnectErr))
			return
		}
	}
	if _, err := s.coyote.owner.Disconnect(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientCoyoteSnapshot(r))
}

// handleCoyoteStrength applies one user step. Raising needs the controlling
// host; lowering is a safety action open to every client that may Stop.
func (s *Server) handleCoyoteStrength(w http.ResponseWriter, r *http.Request) {
	generation := s.coyote.owner.ControlGeneration()
	var request coyoteStrengthRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	channel, ok := coyoteChannel(request.Channel)
	if !ok || request.Delta == 0 || request.Delta > maxCoyoteStep || request.Delta < -maxCoyoteStep {
		writeError(w, http.StatusBadRequest, errCoyoteBadStep)
		return
	}
	if request.Delta > 0 {
		if _, err := s.requestStopSequence(r); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		if !s.requireController(w, r) {
			return
		}
		if !s.capabilities(r).ConfigureHost {
			writeError(w, http.StatusForbidden, errCoyoteHostOnly)
			return
		}
	}
	if _, err := s.coyote.owner.AdjustStrengthAt(r.Context(), generation, channel, request.Delta); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientCoyoteSnapshot(r))
}

// A calibration waveform is an explicit host-only output action, never a side
// effect of changing strength. Both the global Stop and owner generation fence
// requests delayed in the browser, admission lane or body reader.
func (s *Server) handleCoyoteTest(w http.ResponseWriter, r *http.Request) {
	generation := s.coyote.owner.ControlGeneration()
	if !s.requireController(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errCoyoteHostOnly)
		return
	}
	var request struct {
		Channel string `json:"channel"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	channel, ok := coyoteChannel(request.Channel)
	if !ok || strings.TrimSpace(r.Header.Get(stopSequenceHeader)) == "" {
		writeError(w, http.StatusBadRequest, errors.New("a Coyote channel test needs a channel and the current Stop sequence"))
		return
	}
	if _, err := s.requestStopSequence(r); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	if engine := s.currentMotionEngine(); engine != nil {
		state := engine.Snapshot()
		if state.Running || state.Starting || state.Completing {
			writeError(w, http.StatusConflict, errors.New("pause playback before testing a Coyote channel"))
			return
		}
	}
	if _, err := s.coyote.owner.TestChannel(r.Context(), generation, channel); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientCoyoteSnapshot(r))
}

// handleCoyoteZero sets both channels to zero. Like Stop, it needs no
// controller admission.
func (s *Server) handleCoyoteZero(w http.ResponseWriter, r *http.Request) {
	finishUnreadBody(w, r)
	if err := s.coyote.owner.EmergencyZero(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientCoyoteSnapshot(r))
}

func coyoteChannel(name string) (estim.Channel, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "a":
		return estim.ChannelA, true
	case "b":
		return estim.ChannelB, true
	default:
		return 0, false
	}
}

func (s *Server) coyoteSnapshot() coyoteSnapshot {
	settings, _ := s.store.Snapshot()
	status := s.coyote.owner.Status()
	return coyoteSnapshot{
		Output:    settings.Device.Output,
		Available: status.Simulated || s.coyote.helperPath != "",
		Status:    status,
	}
}

// clientCoyoteSnapshot hides the hardware address and error details from
// clients that cannot configure the host.
func (s *Server) clientCoyoteSnapshot(r *http.Request) coyoteSnapshot {
	snapshot := s.coyoteSnapshot()
	if !s.capabilities(r).ConfigureHost {
		snapshot.Status.Address = ""
		snapshot.Status.LastError = clientFailure(snapshot.Status.LastError)
	}
	return snapshot
}

// coyoteMotionTransport returns the owner for an engine that drives the
// Coyote. Motion never starts against a disconnected Coyote: that would be a
// silent fallback to no output.
func (s *Server) coyoteMotionTransport() (transport.Transport, error) {
	return s.coyote.owner.MotionTransport()
}

// startCoyoteZero runs the Coyote half of a global Stop beside the engine
// Stop and returns a wait function.
func (s *Server) startCoyoteZero() func() error {
	return s.coyote.owner.BeginEmergencyStop()
}

// applyCoyoteSettingsTransition applies live Coyote changes and disconnects
// when the Coyote leaves the selected output.
func (s *Server) applyCoyoteSettingsTransition(previous, next config.Settings) error {
	if previous.Device.HasDevice(config.DeviceOutputCoyote) && !next.Device.HasDevice(config.DeviceOutputCoyote) {
		_, err := s.coyote.owner.Disconnect()
		return err
	}
	var zeroErr error
	if config.UsesCoyote(previous.Device.Output) && !config.UsesCoyote(next.Device.Output) {
		zeroErr = s.coyote.owner.EmergencyZero()
	}
	if previous.Estim != next.Estim {
		return errors.Join(zeroErr, s.coyote.owner.Configure(coyoteConfig(next.Estim)))
	}
	return zeroErr
}

func (s *Server) closeCoyote() {
	if s.coyote.owner != nil {
		_ = s.coyote.owner.Close()
	}
}
