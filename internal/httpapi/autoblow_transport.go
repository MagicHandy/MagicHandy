package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	autoblowConnectTimeout = 30 * time.Second
	// autoblowSpeedStep is how far a device speed button moves the Speed
	// limit's maximum.
	autoblowSpeedStep = 10
)

var (
	errAutoblowUnknownDevice = errors.New("name the Autoblow Ultra or the VacuGlide")
	errAutoblowNotAdded      = errors.New("add the device before connecting it")
	errAutoblowNoToken       = errors.New("save the device token first")
	errAutoblowHostOnly      = errors.New("only the controlling administrator can connect, test or adjust the stroke of this device")
	errAutoblowBadDirection  = errors.New("the stroke step must be longer or shorter")
)

// autoblowRuntime holds Autoblow's two cloud devices (ADR 0069). Each has its
// own token; nothing is shared but the cloud client code.
type autoblowRuntime struct {
	ultra     *transport.AutoblowUltra
	vacuglide *transport.VacuGlide
	// buttons tracks work a device button started, so shutdown waits for it.
	buttons sync.WaitGroup
}

// autoblowDeviceOwner is what the handlers need from either owner.
type autoblowDeviceOwner interface {
	Connect(context.Context) (transport.AutoblowStatus, error)
	Disconnect() (transport.AutoblowStatus, error)
	Configure(transport.AutoblowConfig)
	ControlGeneration() uint64
	TestAt(uint64) error
}

type autoblowSnapshot struct {
	Ultra     transport.AutoblowStatus `json:"autoblow"`
	VacuGlide transport.AutoblowStatus `json:"vacuglide"`
}

// configureAutoblow builds both owners once. Simulation points the client at
// the in-process imitation of Autoblow's cloud.
func (s *Server) configureAutoblow(runtime Runtime, settings config.Settings) {
	client := runtime.AutoblowClient
	if client.HTTPClient == nil {
		cloud := http.DefaultTransport.(*http.Transport).Clone()
		cloud.TLSHandshakeTimeout = 10 * time.Second
		cloud.ResponseHeaderTimeout = 15 * time.Second
		client.HTTPClient = &http.Client{Transport: cloud}
	}
	options := transport.AutoblowOptions{Client: client, Simulated: runtime.AutoblowSimulated, OnButton: s.autoblowButton}
	s.autoblow.ultra = transport.NewAutoblowUltra(options)
	s.autoblow.vacuglide = transport.NewVacuGlide(options)
	s.autoblow.ultra.Configure(ultraConfig(settings))
	s.autoblow.vacuglide.Configure(vacuglideConfig(settings))
}

func ultraConfig(settings config.Settings) transport.AutoblowConfig {
	return transport.AutoblowConfig{Token: settings.Autoblow.DeviceToken, OffsetMillis: settings.Autoblow.TimingOffsetMillis}
}

// vacuglideConfig converts saved settings: the Tempo range the owner plays
// tempo across, and whether motion may step the valves (ADR 0071).
func vacuglideConfig(settings config.Settings) transport.AutoblowConfig {
	return transport.AutoblowConfig{Token: settings.VacuGlide.DeviceToken, OffsetMillis: settings.VacuGlide.TimingOffsetMillis,
		TempoMinPercent: float64(settings.VacuGlide.TempoMinPercent), TempoMaxPercent: float64(settings.VacuGlide.TempoMaxPercent),
		MotionAir: settings.VacuGlide.MotionAir}
}

// deviceRoutes registers the devices that own their connections outside the
// Handy and Intiface paths: the Coyote and Autoblow's cloud devices.
func (s *Server) deviceRoutes(mux *http.ServeMux) {
	s.coyoteRoutes(mux)
	s.autoblowRoutes(mux)
	s.tempoRoutes(mux)
}

func (s *Server) autoblowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/transport/autoblow/status", s.handleAutoblowStatus)
	mux.HandleFunc("POST /api/transport/autoblow/connect", s.handleAutoblowConnect)
	mux.HandleFunc("POST /api/transport/autoblow/disconnect", s.handleAutoblowDisconnect)
	mux.HandleFunc("POST /api/transport/autoblow/test", s.handleAutoblowTest)
	mux.HandleFunc("POST /api/transport/autoblow/stroke", s.handleVacuGlideStroke)
	mux.HandleFunc("PUT /api/settings/device/autoblow-token", s.handlePutAutoblowToken)
}

func (s *Server) autoblowOwner(device string) (autoblowDeviceOwner, bool) {
	switch strings.TrimSpace(device) {
	case config.DeviceAutoblow:
		return s.autoblow.ultra, true
	case config.DeviceVacuGlide:
		return s.autoblow.vacuglide, true
	default:
		return nil, false
	}
}

func (s *Server) handleAutoblowStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.clientAutoblowSnapshot(r))
}

func (s *Server) handleAutoblowConnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errAutoblowHostOnly)
		return
	}
	var request struct {
		Device string `json:"device"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	owner, ok := s.autoblowOwner(request.Device)
	if !ok {
		writeError(w, http.StatusBadRequest, errAutoblowUnknownDevice)
		return
	}
	settings, _ := s.store.Snapshot()
	switch {
	case !settings.Device.HasDevice(request.Device):
		writeError(w, http.StatusConflict, errAutoblowNotAdded)
		return
	case settings.AutoblowToken(request.Device) == "":
		writeError(w, http.StatusConflict, errAutoblowNoToken)
		return
	}
	if request.Device == config.DeviceVacuGlide {
		owner.Configure(vacuglideConfig(settings))
	} else {
		owner.Configure(ultraConfig(settings))
	}
	ctx, cancel := context.WithTimeout(r.Context(), autoblowConnectTimeout)
	defer cancel()
	if _, err := owner.Connect(ctx); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientAutoblowSnapshot(r))
}

func (s *Server) handleAutoblowDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		Device string `json:"device"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	owner, ok := s.autoblowOwner(request.Device)
	if !ok {
		writeError(w, http.StatusBadRequest, errAutoblowUnknownDevice)
		return
	}
	stopErr := s.stopForAutoblowDisconnect(r.Context(), request.Device)
	if _, err := owner.Disconnect(); err != nil || stopErr != nil {
		writeError(w, http.StatusBadGateway, errors.Join(stopErr, err))
		return
	}
	writeJSON(w, http.StatusOK, s.clientAutoblowSnapshot(r))
}

// stopForAutoblowDisconnect ends a run that includes the device, so a
// disconnect never leaves the others running against a lost device.
func (s *Server) stopForAutoblowDisconnect(ctx context.Context, device string) error {
	settings, _ := s.store.Snapshot()
	if !settings.Device.Plays(device) {
		return nil
	}
	reason := device + "_disconnected"
	if s.modes != nil {
		s.modes.Stop(reason)
	}
	return s.stopAndClearMotionEngine(ctx, reason)
}

// handleAutoblowTest runs a short, gentle test while motion is idle. It is an
// explicit host action fenced by the current Stop sequence and the owner's
// control generation, like the vibrator's test.
func (s *Server) handleAutoblowTest(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Device string `json:"device"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	owner, ok := s.autoblowOwner(request.Device)
	if !ok {
		writeError(w, http.StatusBadRequest, errAutoblowUnknownDevice)
		return
	}
	generation := owner.ControlGeneration()
	if !s.requireController(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errAutoblowHostOnly)
		return
	}
	if !s.requireAutoblowStopSequence(w, r) {
		return
	}
	if engine := s.currentMotionEngine(); engine != nil {
		if state := engine.Snapshot(); state.Running || state.Starting || state.Completing {
			writeError(w, http.StatusConflict, errors.New("pause playback before testing the device"))
			return
		}
	}
	if err := owner.TestAt(generation); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientAutoblowSnapshot(r))
}

// Both stroke valves change an unmeasured pneumatic state. Neither is an
// emergency reduction: both need the controlling administrator and a current
// Stop sequence. Emergency Stop closes valves without these restrictions.
func (s *Server) handleVacuGlideStroke(w http.ResponseWriter, r *http.Request) {
	generation := s.autoblow.vacuglide.ControlGeneration()
	var request struct {
		Direction string `json:"direction"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	longer := false
	switch request.Direction {
	case "longer":
		longer = true
	case "shorter":
	default:
		writeError(w, http.StatusBadRequest, errAutoblowBadDirection)
		return
	}
	if !s.requireController(w, r) || !s.requireAutoblowStopSequence(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errAutoblowHostOnly)
		return
	}
	if err := s.autoblow.vacuglide.StrokeStepAt(generation, longer); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, s.clientAutoblowSnapshot(r))
}

func (s *Server) requireAutoblowStopSequence(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get(stopSequenceHeader)) == "" {
		writeError(w, http.StatusBadRequest, errors.New("this needs the current Stop sequence"))
		return false
	}
	if _, err := s.requestStopSequence(r); err != nil {
		writeError(w, http.StatusConflict, err)
		return false
	}
	return true
}

// handlePutAutoblowToken saves a device token with its own explicit Save,
// like the Handy connection key. A changed token disconnects the device.
func (s *Server) handlePutAutoblowToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var update struct {
		Device string `json:"device"`
		Token  string `json:"token"`
	}
	if err := decodeJSON(r, &update); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.autoblowOwner(update.Device); !ok {
		writeError(w, http.StatusBadRequest, errAutoblowUnknownDevice)
		return
	}
	if strings.TrimSpace(update.Token) == "" {
		writeError(w, http.StatusBadRequest, errors.New("a device token is required"))
		return
	}
	if _, err := config.DefaultSettings().WithAutoblowToken(update.Device, update.Token); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		return current.WithAutoblowToken(update.Device, update.Token)
	})
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, errors.New("the device token could not be saved"))
		return
	}
	payload := map[string]any{"settings": s.clientSettings(r, saved.Public())}
	status := http.StatusOK
	if runtimeErr != nil {
		status = http.StatusBadGateway
		payload["error"] = "the device token was saved, but the device runtime could not be updated"
	}
	writeJSON(w, status, payload)
}

func (s *Server) autoblowSnapshot() autoblowSnapshot {
	return autoblowSnapshot{Ultra: s.autoblow.ultra.Status(), VacuGlide: s.autoblow.vacuglide.Status()}
}

// clientAutoblowSnapshot hides the cluster and error details from clients
// that cannot configure the host.
func (s *Server) clientAutoblowSnapshot(r *http.Request) autoblowSnapshot {
	snapshot := s.autoblowSnapshot()
	if !s.capabilities(r).ConfigureHost {
		for _, status := range []*transport.AutoblowStatus{&snapshot.Ultra, &snapshot.VacuGlide} {
			status.Cluster = ""
			status.LastError = clientFailure(status.LastError)
		}
	}
	return snapshot
}

// autoblowMotionTransport returns an engine-bound session. Motion never
// starts against a disconnected device: that would be a silent fallback.
func (s *Server) autoblowMotionTransport(device string) (transport.Transport, error) {
	if device == config.DeviceVacuGlide {
		return s.autoblow.vacuglide.MotionTransport()
	}
	return s.autoblow.ultra.MotionTransport()
}

// autoblowEngineSuffix names the connections an engine was built for.
func (s *Server) autoblowEngineSuffix(device config.DeviceSettings) string {
	suffix := ""
	if device.Plays(config.DeviceAutoblow) {
		suffix += fmt.Sprintf("/autoblow-%d", s.autoblow.ultra.SessionID())
	}
	if device.Plays(config.DeviceVacuGlide) {
		suffix += fmt.Sprintf("/vacuglide-%d", s.autoblow.vacuglide.SessionID())
	}
	return suffix
}

// startAutoblowEmergencyStop stops both devices beside the engine's Stop,
// whether or not they play: Stop always reaches every connected device. It
// travels through Autoblow's cloud, the only path to them.
func (s *Server) startAutoblowEmergencyStop() func() error {
	waitUltra := s.autoblow.ultra.BeginEmergencyStop()
	waitVacuGlide := s.autoblow.vacuglide.BeginEmergencyStop()
	return func() error { return errors.Join(waitUltra(), waitVacuGlide()) }
}

// applyAutoblowSettingsTransition disconnects a removed device or one whose
// token changed, stopping a run that includes it first, and applies offsets
// and the VacuGlide's speed and stroke choices live.
func (s *Server) applyAutoblowSettingsTransition(ctx context.Context, previous, next config.Settings) error {
	var errs []error
	for _, device := range []string{config.DeviceAutoblow, config.DeviceVacuGlide} {
		owner, _ := s.autoblowOwner(device)
		removed := previous.Device.HasDevice(device) && !next.Device.HasDevice(device)
		if removed || previous.AutoblowToken(device) != next.AutoblowToken(device) {
			if previous.Device.Plays(device) && next.Device.Plays(device) {
				errs = append(errs, s.stopForAutoblowDisconnect(ctx, device))
			}
			if _, err := owner.Disconnect(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	s.autoblow.ultra.Configure(ultraConfig(next))
	s.autoblow.vacuglide.Configure(vacuglideConfig(next))
	return errors.Join(errs...)
}

// autoblowButton acts on a device button. While a device is online its
// buttons do nothing by themselves (Autoblow's API sends them as events), so
// the pause or mode button stops everything, like Esc. The Ultra's speed
// buttons nudge the Speed limit's maximum, like the device panel; the
// VacuGlide's move the top of its own Tempo range (ADR 0071).
func (s *Server) autoblowButton(device, button string) {
	if s.quiescing.Load() {
		return
	}
	switch autoblowButtonAction(device, button) {
	case "stop":
		s.autoblow.buttons.Add(1)
		go func() {
			defer s.autoblow.buttons.Done()
			if _, err := s.emergencyStop(context.Background(), "device_button"); err != nil {
				s.logger.Warn("device button Stop could not confirm every device", "device", device, "error", err)
			}
		}()
	case "faster", "slower":
		step := autoblowSpeedStep
		if autoblowButtonAction(device, button) == "slower" {
			step = -autoblowSpeedStep
		}
		s.autoblow.buttons.Add(1)
		go func() {
			defer s.autoblow.buttons.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _, err, refreshErr := s.updateSettingsAndRuntime(ctx, func(current config.Settings) (config.Settings, error) {
				if device == transport.VacuGlideDevice {
					tempo := &current.VacuGlide
					delta := config.VacuGlideTempoButtonStep
					if step < 0 {
						delta = -delta
					}
					tempo.TempoMaxPercent = max(tempo.TempoMinPercent, min(100, tempo.TempoMaxPercent+delta))
					return current, nil
				}
				motionSettings := current.Motion
				motionSettings.SpeedMaxPercent = max(motionSettings.SpeedMinPercent, min(100, motionSettings.SpeedMaxPercent+step))
				current.Motion = motionSettings
				return current, nil
			})
			if err = errors.Join(err, refreshErr); err != nil {
				s.logger.Warn("device speed button could not change the Speed limit", "device", device, "error", err)
			}
		}()
	}
}

// autoblowButtonAction maps a device's button to what MagicHandy does.
func autoblowButtonAction(device, button string) string {
	switch {
	case device == transport.AutoblowUltraDevice && button == autoblow.ButtonUltraPause,
		device == transport.VacuGlideDevice && button == autoblow.ButtonVacuGlideMode:
		return "stop"
	case button == autoblow.ButtonUltraSpeedUp || button == autoblow.ButtonVacuGlideSpeedPlus:
		return "faster"
	case button == autoblow.ButtonUltraSpeedDown || button == autoblow.ButtonVacuGlideSpeedMinus:
		return "slower"
	default:
		return ""
	}
}

func (s *Server) closeAutoblow() {
	if s.autoblow.ultra != nil {
		_ = s.autoblow.ultra.Close()
	}
	if s.autoblow.vacuglide != nil {
		_ = s.autoblow.vacuglide.Close()
	}
	s.autoblow.buttons.Wait()
}
