package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// intifaceRuntime holds the one Intiface Central connection. Intiface serves
// a single client, so every device reached through it shares this hub. Each
// connected device holds a lease; the hub closes when the last lease ends,
// or when its address changes or the server shuts down.
type intifaceRuntime struct {
	opMu        sync.Mutex
	mu          sync.Mutex
	hub         *transport.IntifaceHub
	leases      map[string]bool
	httpClient  *http.Client
	status      transport.IntifaceStatus
	diagnostics transport.TransportDiagnostics
}

type intifaceSnapshot struct {
	DispatchOwner string                         `json:"dispatch_owner"`
	Address       string                         `json:"address"`
	Status        transport.IntifaceStatus       `json:"status"`
	Diagnostics   transport.TransportDiagnostics `json:"diagnostics"`
	// Leases names the devices holding the shared connection.
	Leases []string `json:"leases"`
	// PingWatchdog reports whether Intiface stops its devices when this app
	// stops answering its keepalive.
	PingWatchdog bool `json:"ping_watchdog"`
}

func newIntifaceRuntime(runtime Runtime) intifaceRuntime {
	return intifaceRuntime{
		httpClient: runtime.IntifaceHTTPClient,
		leases:     map[string]bool{},
		status: transport.IntifaceStatus{
			PlaybackState: "idle",
			Devices:       []transport.IntifaceDevice{},
		},
		diagnostics: transport.TransportDiagnostics{
			Name:          "intiface_buttplug_v3",
			PlaybackState: "idle",
		},
	}
}

func (s *Server) intifaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/transport/intiface/status", s.handleIntifaceStatus)
	mux.HandleFunc("POST /api/transport/intiface/connect", s.handleIntifaceConnect)
	mux.HandleFunc("POST /api/transport/intiface/disconnect", s.handleIntifaceDisconnect)
	mux.HandleFunc("POST /api/transport/intiface/scan", s.handleIntifaceStartScan)
	mux.HandleFunc("DELETE /api/transport/intiface/scan", s.handleIntifaceStopScan)
	mux.HandleFunc("POST /api/transport/intiface/select", s.handleIntifaceSelect)
	mux.HandleFunc("GET /api/transport/intiface/diagnostics", s.handleIntifaceDiagnostics)
}

func (s *Server) handleIntifaceStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.clientIntifaceSnapshot(r))
}

func (s *Server) handleIntifaceDiagnostics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.intifaceSnapshot().Diagnostics)
}

// intifaceLeaseRequest names the device a connect or disconnect is for. An
// empty body means the Handy, as older clients send.
type intifaceLeaseRequest struct {
	Device string `json:"device"`
}

func decodeIntifaceLease(r *http.Request) (string, error) {
	var body intifaceLeaseRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			return "", err
		}
	}
	if body.Device == "" {
		body.Device = config.DeviceOutputHandy
	}
	if !slices.Contains(intifaceLeaseDevices, body.Device) {
		return "", errors.New("this device does not connect through Intiface")
	}
	return body.Device, nil
}

// intifaceLeaseDevices are the devices that can reach Intiface Central.
var intifaceLeaseDevices = []string{config.DeviceOutputHandy}

func (s *Server) handleIntifaceConnect(w http.ResponseWriter, r *http.Request) {
	device, err := decodeIntifaceLease(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.requireIntifaceInventory(w, device) || !s.requireController(w, r) {
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	settings, _ := s.store.Snapshot()
	if device == config.DeviceOutputHandy && settings.Device.HSPDispatchOwner != config.DispatchOwnerIntiface {
		writeError(w, http.StatusConflict, errors.New("select and save the Intiface dispatch owner before connecting"))
		return
	}
	s.intiface.mu.Lock()
	existing := s.intiface.hub
	connected := existing != nil && existing.Status().Connected
	held := s.intiface.leases[device]
	s.intiface.mu.Unlock()
	if connected && held {
		writeError(w, http.StatusConflict, errors.New("intiface is already connected"))
		return
	}
	if !connected {
		if err := s.openIntifaceHub(r.Context(), settings.Device.IntifaceServerAddress); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	s.intiface.mu.Lock()
	s.intiface.leases[device] = true
	s.intiface.mu.Unlock()
	writeJSON(w, http.StatusOK, s.intifaceSnapshot())
}

// requireIntifaceInventory checks that device is saved before it connects.
func (s *Server) requireIntifaceInventory(w http.ResponseWriter, device string) bool {
	_ = device
	return s.requireHandyConnection(w)
}

// openIntifaceHub replaces a stale hub with a newly connected one. The
// caller holds opMu.
func (s *Server) openIntifaceHub(ctx context.Context, address string) error {
	s.closeIntifaceSession()
	hub, err := transport.NewIntifaceHub(transport.IntifaceOptions{
		Address:    address,
		HTTPClient: s.intiface.httpClient,
	})
	if err != nil {
		return err
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = hub.Connect(connectCtx)
	cancel()
	if err != nil {
		_ = hub.Close()
		s.logger.Warn("Intiface connection failed", "error", err)
		return errors.New("intiface Central is not reachable; confirm it is running and the saved server address is correct")
	}
	s.intiface.mu.Lock()
	s.intiface.hub = hub
	s.intiface.leases = map[string]bool{}
	s.intiface.mu.Unlock()
	return nil
}

func (s *Server) handleIntifaceDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	device, err := decodeIntifaceLease(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()

	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
	defer cancel()
	var stopErr error
	if device == config.DeviceOutputHandy {
		stopErr = s.stopForHandyDisconnect(stopCtx, "intiface_disconnected")
	}
	s.writeIntifaceResult(w, errors.Join(stopErr, s.releaseIntifaceLeaseLocked(stopCtx, device)))
}

// releaseIntifaceLease ends one device's use of the shared connection.
func (s *Server) releaseIntifaceLease(ctx context.Context, device string) error {
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	return s.releaseIntifaceLeaseLocked(ctx, device)
}

// releaseIntifaceLeaseLocked stops and forgets the device's selection, and
// closes the hub when no device holds it any more. The caller holds opMu.
func (s *Server) releaseIntifaceLeaseLocked(ctx context.Context, device string) error {
	s.intiface.mu.Lock()
	hub := s.intiface.hub
	delete(s.intiface.leases, device)
	remaining := len(s.intiface.leases)
	s.intiface.mu.Unlock()
	if hub == nil {
		return nil
	}
	if remaining == 0 {
		s.closeIntifaceSession()
		return nil
	}
	if device == config.DeviceOutputHandy {
		return hub.Linear().Release(ctx)
	}
	return nil
}

func (s *Server) handleIntifaceStartScan(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	hub, err := s.currentIntifaceHub()
	if err == nil {
		err = hub.StartScanning(r.Context())
	}
	s.writeIntifaceResult(w, err)
}

func (s *Server) handleIntifaceStopScan(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	hub, err := s.currentIntifaceHub()
	if err == nil {
		err = hub.StopScanning(r.Context())
	}
	s.writeIntifaceResult(w, err)
}

func (s *Server) handleIntifaceSelect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	var body struct {
		DeviceIndex   uint32 `json:"device_index"`
		ActuatorIndex uint32 `json:"actuator_index"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	finishStop, _ := s.beginGlobalStop("intiface_selection_changed", r.Context())
	defer finishStop()

	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
	defer cancel()
	err := s.stopAndClearMotionEngine(stopCtx, "intiface_selection_changed")
	hub, hubErr := s.currentIntifaceHub()
	if err == nil {
		err = hubErr
	}
	if err == nil {
		// Select stops both the previous and the new device before it is ready.
		err = hub.Linear().Select(stopCtx, body.DeviceIndex, body.ActuatorIndex)
	}
	if err == nil && hub.Status().Scanning {
		// The search is done once an actuator is chosen, and a scan left
		// running keeps the Bluetooth radio busy beside the live connection.
		// Best effort: the selection already succeeded either way.
		scanCtx, cancelScan := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		if scanErr := hub.StopScanning(scanCtx); scanErr != nil {
			s.logger.Warn("stop Intiface scan after device selection", "error", scanErr)
		}
		cancelScan()
	}
	s.writeIntifaceResult(w, err)
}

func (s *Server) writeIntifaceResult(w http.ResponseWriter, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, s.intifaceSnapshot())
}

func (s *Server) currentIntifaceHub() (*transport.IntifaceHub, error) {
	s.intiface.mu.Lock()
	defer s.intiface.mu.Unlock()
	if s.intiface.hub == nil || !s.intiface.hub.Status().Connected {
		return nil, errors.New("intiface is not connected")
	}
	return s.intiface.hub, nil
}

// currentIntiface returns the stroker output of a connected hub.
func (s *Server) currentIntiface() (*transport.Intiface, error) {
	hub, err := s.currentIntifaceHub()
	if err != nil {
		return nil, err
	}
	return hub.Linear(), nil
}

func (s *Server) intifaceSnapshot() intifaceSnapshot {
	settings, _ := s.store.Snapshot()
	s.intiface.mu.Lock()
	defer s.intiface.mu.Unlock()
	snapshot := intifaceSnapshot{
		DispatchOwner: settings.Device.HSPDispatchOwner,
		Address:       settings.Device.IntifaceServerAddress,
		Status:        s.intiface.status,
		Diagnostics:   s.intiface.diagnostics,
		Leases:        []string{},
	}
	if hub := s.intiface.hub; hub != nil {
		snapshot.Status = hub.Linear().Status()
		snapshot.Diagnostics = hub.Linear().Diagnostics()
		snapshot.PingWatchdog = hub.Status().PingWatchdog
		for _, device := range intifaceLeaseDevices {
			if s.intiface.leases[device] {
				snapshot.Leases = append(snapshot.Leases, device)
			}
		}
	}
	if snapshot.Status.Devices == nil {
		snapshot.Status.Devices = []transport.IntifaceDevice{}
	}
	return snapshot
}

// startIntifaceEmergencyStop runs the Intiface half of a global Stop beside
// the engine Stop: StopAllDevices on a live session, behind the hub's write
// barrier, whatever the engine or the device selections are. It returns a
// wait function.
func (s *Server) startIntifaceEmergencyStop() func() error {
	s.intiface.mu.Lock()
	hub := s.intiface.hub
	s.intiface.mu.Unlock()
	if hub == nil || !hub.Status().Connected {
		return func() error { return nil }
	}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		done <- hub.EmergencyStop(ctx)
	}()
	return func() error { return <-done }
}

func (s *Server) closeIntiface() {
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	s.closeIntifaceSession()
}

// closeIntifaceSession closes the hub and every device's use of it. The
// caller holds opMu.
func (s *Server) closeIntifaceSession() {
	s.intiface.mu.Lock()
	hub := s.intiface.hub
	s.intiface.hub = nil
	s.intiface.leases = map[string]bool{}
	s.intiface.mu.Unlock()
	if hub != nil {
		_ = hub.Close()
		s.intiface.mu.Lock()
		s.intiface.status = hub.Linear().Status()
		s.intiface.diagnostics = hub.Linear().Diagnostics()
		s.intiface.mu.Unlock()
	}
}
