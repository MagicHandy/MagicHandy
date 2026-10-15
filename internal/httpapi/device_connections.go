package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (s *Server) handlePutDeviceConnection(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		Device string `json:"device"`
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, _ := s.store.Snapshot()
	if _, err := settings.Device.ChangeConnection(request.Device, request.Action); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.writeImmediateSettings(w, r, func(settings config.Settings) (config.Settings, error) {
		var err error
		settings.Device, err = settings.Device.ChangeConnection(request.Device, request.Action)
		return settings, err
	})
}

// An engine-bound Stop cannot reach a prepared Handy excluded from that engine,
// or a new BLE connection that replaced its old link. Global Stop covers it too.
func (s *Server) stopHandyOutsideEngine(ctx context.Context, engineOwner, reason string) error {
	settings, _ := s.store.Snapshot()
	if !settings.Device.HasDevice(config.DeviceOutputHandy) {
		return nil
	}
	outside := !engineOwnerPlays(engineOwner, config.DeviceOutputHandy)
	connected := false
	switch settings.Device.HSPDispatchOwner {
	case config.DispatchOwnerNativeBluetooth:
		connected = s.nativeBluetooth.owner.Status().Connected
		outside = outside || !strings.HasSuffix(engineOwner, fmt.Sprintf("/native-ble-%d", s.nativeBluetooth.owner.SessionID()))
	case config.DispatchOwnerBrowserBluetooth:
		connected = s.bluetooth.bridge.Snapshot().Ready
		outside = outside || !strings.HasSuffix(engineOwner, fmt.Sprintf("/ble-%d", s.bluetooth.bridge.SessionID()))
	case config.DispatchOwnerIntiface:
		if owner, err := s.currentIntiface(); err == nil {
			connected = owner.Status().Connected
		}
	case config.DispatchOwnerCloudREST:
		connected = s.cloudDiagnostics().Connected
	}
	if !outside || !connected {
		return nil
	}
	owner, err := s.newHandyStopTransport(settings)
	if err != nil {
		return err
	}
	_, err = owner.Stop(ctx, transport.StopCommand{Reason: reason})
	return err
}

// Removing a saved connection retires its dispatch owner even when excluded
// from playback. Excluding alone leaves the connection prepared and silent.
func (s *Server) removeHandyConnection(ctx context.Context, previous config.Settings) error {
	var stopErr error
	switch previous.Device.HSPDispatchOwner {
	case config.DispatchOwnerNativeBluetooth:
		return s.nativeBluetooth.owner.Disconnect(ctx)
	case config.DispatchOwnerCloudREST:
		if s.cloudDiagnostics().Connected && !s.motion.simulated {
			owner, err := s.newCloudTransportForControl()
			if err == nil {
				_, err = owner.Stop(ctx, transport.StopCommand{Reason: "connection_removed"})
			}
			stopErr = err
		}
		s.setCloudControlReleased(true)
	case config.DispatchOwnerIntiface:
		stopErr = s.releaseIntifaceLease(ctx, config.DeviceOutputHandy)
	case config.DispatchOwnerBrowserBluetooth:
		if s.bluetooth.bridge.Snapshot().Ready {
			owner, err := s.newBluetoothTransport()
			if err == nil {
				_, err = owner.Stop(ctx, transport.StopCommand{Reason: "connection_removed"})
			}
			stopErr = err
		}
		s.bluetooth.gateway.mu.Lock()
		if lease := s.bluetooth.gateway.lease; lease != nil {
			lease.cancel()
			s.bluetooth.gateway.lease = nil
		}
		s.bluetooth.bridge.DisconnectClient("", "Connection removed.")
		s.bluetooth.gateway.mu.Unlock()
	}
	return stopErr
}

// Only an included Handy belongs to this run. Disconnecting a prepared,
// excluded connection must not interrupt Coyote-only playback.
func (s *Server) stopForHandyDisconnect(ctx context.Context, reason string) (err error) {
	settings, _ := s.store.Snapshot()
	if !settings.Device.Plays(config.DeviceOutputHandy) {
		return nil
	}
	finish, _ := s.beginGlobalStop(reason, ctx)
	defer finish()
	if settings.Device.Plays(config.DeviceOutputCoyote) {
		wait := s.startCoyoteZero()
		defer func() { err = errors.Join(err, wait()) }()
	}
	return s.stopAndClearMotionEngine(ctx, reason)
}

func (s *Server) requireHandyConnection(w http.ResponseWriter) bool {
	settings, _ := s.store.Snapshot()
	if settings.Device.HasDevice(config.DeviceOutputHandy) {
		return true
	}
	writeError(w, http.StatusConflict, errors.New("add a Handy connection before connecting it"))
	return false
}
