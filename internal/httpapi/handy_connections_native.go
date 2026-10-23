package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type handyCandidate struct {
	address, deviceID string
	expires           time.Time
}
type handyDiscoveryChoice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	RSSI  int    `json:"rssi"`
	InUse bool   `json:"in_use"`
}

func handyAddressInUse(settings config.Settings, id, address string) bool {
	if id != config.DeviceOutputHandy && strings.EqualFold(settings.Device.NativeBluetoothAddress, address) && address != "" {
		return true
	}
	for _, c := range settings.Device.HandyConnections {
		if c.ID != id && strings.EqualFold(c.NativeBluetoothAddress, address) && address != "" {
			return true
		}
	}
	return false
}

func (s *Server) handleDiscoverHandy(w http.ResponseWriter, r *http.Request) {
	if !s.requireHandyHost(w, r) {
		return
	}
	id := r.PathValue("id")
	settings, _ := s.store.Snapshot()
	c, extra := settings.Device.HandyConnection(id)
	primary := id == config.DeviceOutputHandy && settings.Device.HasDevice(id)
	if !extra && !primary {
		writeError(w, http.StatusNotFound, errors.New("handy connection does not exist"))
		return
	}
	if (extra && c.Method != config.DispatchOwnerNativeBluetooth) || (primary && settings.Device.HSPDispatchOwner != config.DispatchOwnerNativeBluetooth) {
		writeError(w, http.StatusConflict, errors.New("select Native Bluetooth before finding nearby Handys"))
		return
	}
	s.handys.scanMu.Lock()
	defer s.handys.scanMu.Unlock()
	var candidates []ble.Candidate
	if s.motion.simulated {
		candidates = []ble.Candidate{{Name: "Simulated Handy", Address: "simulated-handy", RSSI: -35}}
	} else {
		discoverer, ok := s.handys.connector.(ble.Discoverer)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, errors.New("native Handy discovery is unavailable"))
			return
		}
		var err error
		candidates, err = discoverer.Discover(r.Context(), 8*time.Second)
		if err != nil {
			writeError(w, http.StatusBadGateway, errors.New("handy discovery failed; check the Bluetooth helper and try again"))
			return
		}
	}
	slices.SortFunc(candidates, func(a, b ble.Candidate) int { return b.RSSI - a.RSSI })
	choices := make([]handyDiscoveryChoice, 0, len(candidates))
	s.handys.mu.Lock()
	defer s.handys.mu.Unlock()
	if s.handys.candidates == nil {
		s.handys.candidates = map[string]handyCandidate{}
	}
	for token, c := range s.handys.candidates {
		if c.deviceID == id || time.Now().After(c.expires) {
			delete(s.handys.candidates, token)
		}
	}
	for _, c := range candidates[:min(len(candidates), 64)] {
		if c.Address == "" {
			continue
		}
		token := rand.Text()
		s.handys.candidates[token] = handyCandidate{address: c.Address, deviceID: id, expires: time.Now().Add(2 * time.Minute)}
		choices = append(choices, handyDiscoveryChoice{ID: token, Name: c.Name, RSSI: c.RSSI, InUse: handyAddressInUse(settings, id, c.Address)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": choices})
}

func (s *Server) handyCandidateAddress(id, token, remembered string) (string, error) {
	if token == "" {
		if remembered == "" {
			return "", errors.New("find nearby Handys and choose a device before connecting")
		}
		return remembered, nil
	}
	s.handys.mu.Lock()
	defer s.handys.mu.Unlock()
	c, ok := s.handys.candidates[token]
	if !ok || c.deviceID != id || time.Now().After(c.expires) {
		return "", errors.New("handy selection expired; find nearby devices again")
	}
	delete(s.handys.candidates, token)
	return c.address, nil
}

func (s *Server) connectNamedHandyNative(ctx context.Context, c config.HandyConnection, token string) (transport.Transport, error) {
	// Serialize native selection through the save, so two names cannot reserve
	// the same physical device before either has remembered its address.
	s.handys.nativeMu.Lock()
	defer s.handys.nativeMu.Unlock()
	address, err := s.handyCandidateAddress(c.ID, token, c.NativeBluetoothAddress)
	if err != nil {
		return nil, err
	}
	settings, _ := s.store.Snapshot()
	if handyAddressInUse(settings, c.ID, address) {
		return nil, errors.New("this Bluetooth device is already saved for another Handy")
	}
	entry := s.handyEntry(c.ID)
	device, err := entry.native.Connect(ctx, ble.ConnectRequest{SelectedAddress: true, RememberedAddress: address, ConnectTimeout: ble.DefaultConnectTimeout, ScanTimeout: 8 * time.Second})
	if err != nil {
		return nil, errors.New("native Handy connection failed; check Bluetooth and try again")
	}
	if entry.native.Status().Simulated {
		return entry.native.NewTransport(transport.HandyBluetoothOptions{ReverseDirection: settings.Motion.ReverseDirection}), nil
	}
	_, _, err = s.store.UpdateContext(ctx, func(current config.Settings) (config.Settings, error) {
		for i, profile := range current.Device.HandyConnections {
			if profile.ID == c.ID {
				if profile.Method != c.Method {
					return current, errors.New("handy method changed while connecting")
				}
				current.Device.HandyConnections[i].NativeBluetoothAddress = device.Address
				return current, nil
			}
		}
		return current, errors.New("handy was removed while connecting")
	})
	if err != nil {
		_ = entry.native.Disconnect(context.Background())
		return nil, errors.New("handy connection could not be saved")
	}
	return entry.native.NewTransport(transport.HandyBluetoothOptions{ReverseDirection: settings.Motion.ReverseDirection}), nil
}

func (s *Server) handlePrimaryHandyChoice(w http.ResponseWriter, r *http.Request) {
	if !s.requireHandyHost(w, r) {
		return
	}
	var body struct {
		CandidateID string `json:"candidate_id"`
	}
	if err := decodeOptionalJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.settingsLifecycleMu.Lock()
	defer s.settingsLifecycleMu.Unlock()
	s.handys.nativeMu.Lock()
	defer s.handys.nativeMu.Unlock()
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerNativeBluetooth || !settings.Device.HasDevice(config.DeviceOutputHandy) {
		writeError(w, http.StatusConflict, errors.New("select Native Bluetooth for the primary Handy first"))
		return
	}
	address, err := s.handyCandidateAddress(config.DeviceOutputHandy, body.CandidateID, settings.Device.NativeBluetoothAddress)
	if err == nil && handyAddressInUse(settings, config.DeviceOutputHandy, address) {
		err = errors.New("this Bluetooth device is already saved for another Handy")
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	device, err := s.nativeBluetooth.owner.Connect(ctx, ble.ConnectRequest{SelectedAddress: true, RememberedAddress: address, ConnectTimeout: ble.DefaultConnectTimeout})
	if err != nil {
		writeError(w, http.StatusBadGateway, errors.New("native Handy connection failed; check Bluetooth and try again"))
		return
	}
	if !s.nativeBluetooth.owner.Status().Simulated {
		_, _, err = s.store.UpdateContext(ctx, func(current config.Settings) (config.Settings, error) {
			current.Device.NativeBluetoothAddress = device.Address
			return current, nil
		})
		if err != nil {
			_ = s.nativeBluetooth.owner.Disconnect(ctx)
			writeError(w, http.StatusBadGateway, errors.New("handy connection could not be saved"))
			return
		}
	}
	writeJSON(w, http.StatusOK, s.nativeBluetoothSnapshot())
}

func (s *Server) handleForgetHandy(w http.ResponseWriter, r *http.Request) {
	if !s.requireHandyHost(w, r) {
		return
	}
	s.settingsLifecycleMu.Lock()
	defer s.settingsLifecycleMu.Unlock()
	id := r.PathValue("id")
	settings, _ := s.store.Snapshot()
	if _, ok := settings.Device.HandyConnection(id); !ok {
		writeError(w, http.StatusNotFound, errors.New("handy connection does not exist"))
		return
	}
	entry := s.handyEntry(id)
	entry.opMu.Lock()
	defer entry.opMu.Unlock()
	if owner, _ := entry.current(); owner != nil {
		writeError(w, http.StatusConflict, errors.New("disconnect this Handy before forgetting its device"))
		return
	}
	_, _, err := s.store.UpdateContext(r.Context(), func(current config.Settings) (config.Settings, error) {
		for i, c := range current.Device.HandyConnections {
			if c.ID == id {
				current.Device.HandyConnections[i].NativeBluetoothAddress = ""
				return current, nil
			}
		}
		return current, errors.New("handy connection does not exist")
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.handyConnectionSnapshot(id))
}
