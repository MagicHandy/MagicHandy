package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type handyConnectionsRuntime struct {
	mu         sync.Mutex
	scanMu     sync.Mutex
	nativeMu   sync.Mutex
	entries    map[string]*handyConnectionRuntime
	connector  ble.Connector
	candidates map[string]handyCandidate
}

type handyConnectionRuntime struct {
	opMu       sync.Mutex
	mu         sync.Mutex
	generation uint64
	connecting bool
	cancel     context.CancelFunc
	owner      transport.Transport
	native     *transport.NativeBluetoothOwner
	lost       bool
}

type handyConnectionState struct {
	ID                    string  `json:"id"`
	Connected             bool    `json:"connected"`
	Connecting            bool    `json:"connecting"`
	Status                string  `json:"status"`
	Message               string  `json:"message,omitempty"`
	LatencyMillis         int64   `json:"latency_ms,omitempty"`
	Simulated             bool    `json:"simulated"`
	NativeAvailable       bool    `json:"native_available"`
	NativeSupported       bool    `json:"native_supported"`
	SelectedDeviceIndex   *uint32 `json:"selected_device_index,omitempty"`
	SelectedActuatorIndex *uint32 `json:"selected_actuator_index,omitempty"`
}

func (s *Server) handyEntry(id string) *handyConnectionRuntime {
	s.handys.mu.Lock()
	defer s.handys.mu.Unlock()
	if s.handys.entries == nil {
		s.handys.entries = map[string]*handyConnectionRuntime{}
	}
	entry := s.handys.entries[id]
	if entry == nil {
		entry = &handyConnectionRuntime{native: transport.NewNativeBluetoothOwner(s.handys.connector)}
		s.handys.entries[id] = entry
	}
	return entry
}

func (e *handyConnectionRuntime) current() (transport.Transport, uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.owner, e.generation
}

func (s *Server) handyConnectionSnapshots() []handyConnectionState {
	settings, _ := s.store.Snapshot()
	result := make([]handyConnectionState, 0, len(settings.Device.HandyConnections))
	for _, c := range settings.Device.HandyConnections {
		result = append(result, s.handyConnectionSnapshot(c.ID))
	}
	return result
}

func (s *Server) handyConnectionSnapshot(id string) handyConnectionState {
	entry := s.handyEntry(id)
	entry.mu.Lock()
	owner, connecting, lost := entry.owner, entry.connecting, entry.lost
	entry.mu.Unlock()
	state := handyConnectionState{ID: id, Connecting: connecting, Status: "disconnected", Simulated: s.motion.simulated,
		NativeAvailable: s.nativeBluetooth.available, NativeSupported: s.nativeBluetooth.supported}
	if owner != nil {
		d := owner.Diagnostics()
		state.Connected, state.LatencyMillis = d.Connected, d.LastLatencyMillis
		if !d.Connected {
			lost = true
		}
		if linear, ok := owner.(*transport.Intiface); ok {
			status := linear.Status()
			state.SelectedDeviceIndex, state.SelectedActuatorIndex = status.SelectedDeviceIndex, status.SelectedActuatorIndex
		}
	}
	switch {
	case connecting:
		state.Status = "connecting"
	case state.Connected:
		state.Status = "connected"
	case lost:
		state.Status = "lost"
	}
	return state
}

func (s *Server) namedHandyTransport(id string) (transport.Transport, error) {
	settings, _ := s.store.Snapshot()
	if _, ok := settings.Device.HandyConnection(id); !ok {
		return nil, errors.New("handy connection does not exist")
	}
	entry := s.handyEntry(id)
	owner, generation := entry.current()
	if owner == nil || !owner.Diagnostics().Connected {
		return nil, errors.New("connect the selected Handy before playback")
	}
	if linear, ok := owner.(*transport.Intiface); ok {
		if linear.Status().SelectedDeviceIndex == nil {
			return nil, errors.New("choose an Intiface device for this Handy before playback")
		}
	}
	return transport.NewLeasedTransport(owner, func() bool {
		current, currentGeneration := entry.current()
		return current == owner && generation == currentGeneration
	}), nil
}

func (s *Server) handleConnectHandy(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == config.DeviceOutputHandy {
		s.handlePrimaryHandyChoice(w, r)
		return
	}
	s.settingsLifecycleMu.Lock()
	defer s.settingsLifecycleMu.Unlock()
	settings, _ := s.store.Snapshot()
	c, ok := settings.Device.HandyConnection(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("handy connection does not exist"))
		return
	}
	if c.Method == config.DispatchOwnerNativeBluetooth && !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("only the host can connect native Bluetooth"))
		return
	}
	var body struct {
		CandidateID string `json:"candidate_id"`
	}
	if err := decodeOptionalJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entry := s.handyEntry(id)
	entry.opMu.Lock()
	defer entry.opMu.Unlock()
	if owner, _ := entry.current(); owner != nil && owner.Diagnostics().Connected {
		writeJSON(w, http.StatusOK, s.handyConnectionSnapshot(id))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	entry.mu.Lock()
	entry.generation++
	generation := entry.generation
	entry.connecting, entry.cancel = true, cancel
	entry.mu.Unlock()
	owner, err := s.connectNamedHandy(ctx, c, body.CandidateID)
	entry.mu.Lock()
	stale := entry.generation != generation || ctx.Err() != nil
	entry.connecting, entry.cancel = false, nil
	if err == nil && !stale {
		entry.owner, entry.lost = owner, false
	}
	entry.mu.Unlock()
	if stale && err == nil {
		err = errors.New("handy connection was canceled")
	}
	if err != nil {
		stop, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.disposeNamedHandy(stop, c.ID, c.Method, owner)
		cancelStop()
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, s.handyConnectionSnapshot(id))
}

func (s *Server) connectNamedHandy(ctx context.Context, c config.HandyConnection, candidate string) (transport.Transport, error) {
	if s.motion.simulated && c.Method == config.DispatchOwnerCloudREST {
		return transport.NewFake(), nil
	}
	settings, _ := s.store.Snapshot()
	switch c.Method {
	case config.DispatchOwnerCloudREST:
		owner, err := transport.NewCloudRESTTransport(transport.CloudPrerequisites{
			ApplicationID: resolveCloudApplicationID(settings), ConnectionKey: c.ConnectionKey, FirmwareMajor: 4, APIMajor: 3, HSPAvailable: true},
			transport.CloudBuildOptions{ReverseDirection: settings.Motion.ReverseDirection}, transport.CloudEndpointConfig{BaseURL: s.cloud.baseURL}, s.cloud.client)
		if err != nil {
			return nil, errors.New("save a valid Handy connection key before connecting")
		}
		check, err := owner.CheckConnection(ctx)
		if err != nil || !check.OK || !check.HSPAvailable {
			return nil, errors.New("the Handy could not be reached through Cloud; check its key and connection")
		}
		return owner, nil
	case config.DispatchOwnerNativeBluetooth:
		return s.connectNamedHandyNative(ctx, c, candidate)
	case config.DispatchOwnerIntiface:
		if s.motion.simulated && s.intiface.simulatorAddress == "" {
			return nil, errors.New("the Intiface simulator is unavailable")
		}
		return s.connectNamedHandyIntiface(ctx, c.ID)
	default:
		return nil, errors.New("unsupported Handy connection method")
	}
}

func (s *Server) handleDisconnectHandy(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.settingsLifecycleMu.Lock()
	defer s.settingsLifecycleMu.Unlock()
	id := r.PathValue("id")
	settings, _ := s.store.Snapshot()
	c, ok := settings.Device.HandyConnection(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("handy connection does not exist"))
		return
	}
	if c.Method == config.DispatchOwnerNativeBluetooth && !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("only the host can disconnect native Bluetooth"))
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
	defer cancel()
	var stopErr error
	if settings.Device.Plays(id) {
		finish, _ := s.beginGlobalStop("handy_disconnected", r.Context())
		defer finish()
		stopErr = s.stopAndClearMotionEngine(ctx, "handy_disconnected")
	}
	err := errors.Join(stopErr, s.disconnectNamedHandy(ctx, id, c.Method))
	if err != nil {
		writeError(w, http.StatusBadGateway, errors.New("handy disconnected; device Stop could not be confirmed"))
		return
	}
	writeJSON(w, http.StatusOK, s.handyConnectionSnapshot(id))
}

func (s *Server) disconnectNamedHandy(ctx context.Context, id, method string) error {
	entry := s.handyEntry(id)
	entry.mu.Lock()
	entry.generation++
	if entry.cancel != nil {
		entry.cancel()
	}
	owner := entry.owner
	entry.owner = nil
	entry.mu.Unlock()
	entry.opMu.Lock()
	defer entry.opMu.Unlock()
	return s.disposeNamedHandy(ctx, id, method, owner)
}

func handyIdentityChanged(a, b config.HandyConnection) bool {
	a.Name, b.Name = "", ""
	return a != b
}

func (s *Server) applyHandyConnectionsTransition(ctx context.Context, previous, next config.Settings) (bool, error) {
	sharedCloudChanged := previous.Device.APIApplicationIDSource != next.Device.APIApplicationIDSource || previous.Device.APIApplicationIDOverride != next.Device.APIApplicationIDOverride || previous.Device.FirmwareAPIRequirement != next.Device.FirmwareAPIRequirement
	sharedIntifaceChanged := previous.Device.IntifaceServerAddress != next.Device.IntifaceServerAddress
	if reflect.DeepEqual(previous.Device.HandyConnections, next.Device.HandyConnections) && !sharedCloudChanged && !sharedIntifaceChanged {
		return false, nil
	}
	var changed []config.HandyConnection
	for _, old := range previous.Device.HandyConnections {
		current, exists := next.Device.HandyConnection(old.ID)
		if !exists || handyIdentityChanged(old, current) || (sharedCloudChanged && old.Method == config.DispatchOwnerCloudREST) || (sharedIntifaceChanged && old.Method == config.DispatchOwnerIntiface) {
			changed = append(changed, old)
		}
	}
	if len(changed) == 0 {
		return false, nil
	}
	if s.modes != nil {
		s.modes.Stop("handy_configuration_changed")
	}
	err := s.stopAndClearMotionEngine(ctx, "handy_configuration_changed")
	for _, old := range changed {
		err = errors.Join(err, s.disconnectNamedHandy(ctx, old.ID, old.Method))
	}
	s.handys.mu.Lock()
	for _, old := range changed {
		if _, exists := next.Device.HandyConnection(old.ID); !exists {
			delete(s.handys.entries, old.ID)
		}
	}
	s.handys.mu.Unlock()
	return true, err
}

func (s *Server) interruptNamedHandys() {
	s.handys.mu.Lock()
	entries := make([]*handyConnectionRuntime, 0, len(s.handys.entries))
	for _, e := range s.handys.entries {
		entries = append(entries, e)
	}
	s.handys.mu.Unlock()
	for _, e := range entries {
		e.mu.Lock()
		if e.cancel != nil {
			e.generation++
			e.cancel()
		}
		e.mu.Unlock()
		e.native.Interrupt()
	}
}

func (s *Server) closeNamedHandys(ctx context.Context) {
	s.interruptNamedHandys()
	settings, _ := s.store.Snapshot()
	for _, c := range settings.Device.HandyConnections {
		_ = s.disconnectNamedHandy(ctx, c.ID, c.Method)
	}
}

func (s *Server) namedHandyOwnerSuffix(device config.DeviceSettings) string {
	result := ""
	for _, id := range device.PlaybackSet() {
		if config.IsHandyConnectionID(id) {
			_, generation := s.handyEntry(id).current()
			result += fmt.Sprintf("/%s-%d", id, generation)
		}
	}
	return result
}
