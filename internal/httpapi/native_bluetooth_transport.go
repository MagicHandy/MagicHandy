package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/workerclient"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const bluetoothWorkerName = "bluetooth-worker"

type nativeBluetoothRuntime struct {
	owner     *transport.NativeBluetoothOwner
	available bool
	supported bool
}

type nativeBluetoothSnapshot struct {
	Available        bool                            `json:"available"`
	Supported        bool                            `json:"supported"`
	RememberedDevice bool                            `json:"remembered_device"`
	Status           transport.NativeBluetoothStatus `json:"status"`
}

func (s *Server) configureNativeBluetooth(app Runtime, logger *slog.Logger) {
	connector := app.NativeBluetoothConnector
	s.nativeBluetooth.supported = runtime.GOOS == "windows" || runtime.GOOS == "linux"
	if connector == nil {
		path := resolveWorkerBinary("", app.ExecutablePath, s.store.DataDir(), bluetoothWorkerName)
		s.nativeBluetooth.available = path != "" && s.nativeBluetooth.supported
		connector = workerclient.NewConnector(workerclient.Options{Path: path, Profile: ble.ProfileHandy, Logger: logger})
	} else {
		s.nativeBluetooth.available, s.nativeBluetooth.supported = true, true
	}
	s.nativeBluetooth.owner = transport.NewNativeBluetoothOwner(connector)
}

func (s *Server) nativeBluetoothSnapshot() nativeBluetoothSnapshot {
	settings, _ := s.store.Snapshot()
	return nativeBluetoothSnapshot{Available: s.nativeBluetooth.available, Supported: s.nativeBluetooth.supported, RememberedDevice: settings.Device.NativeBluetoothAddress != "", Status: s.nativeBluetooth.owner.Status()}
}

func (s *Server) nativeBluetoothRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/transport/native-bluetooth/status", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, s.nativeBluetoothSnapshot()) })
	mux.HandleFunc("POST /api/transport/native-bluetooth/connect", s.handleNativeBluetoothConnect)
	mux.HandleFunc("POST /api/transport/native-bluetooth/disconnect", s.handleNativeBluetoothDisconnect)
	mux.HandleFunc("POST /api/transport/native-bluetooth/forget", s.handleNativeBluetoothForget)
}

func (s *Server) requireNativeBluetoothHost(w http.ResponseWriter, r *http.Request) bool {
	if !s.requireController(w, r) {
		return false
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("only the host can configure native Bluetooth"))
		return false
	}
	return true
}

func (s *Server) handleNativeBluetoothConnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireNativeBluetoothHost(w, r) || !s.requireHandyConnection(w) {
		return
	}
	s.settingsLifecycleMu.Lock()
	defer s.settingsLifecycleMu.Unlock()
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerNativeBluetooth {
		writeError(w, http.StatusConflict, errors.New("select Native Bluetooth before connecting"))
		return
	}
	if !s.nativeBluetooth.available {
		writeError(w, http.StatusServiceUnavailable, ble.ErrHelperMissing)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	device, err := s.nativeBluetooth.owner.Connect(ctx, ble.ConnectRequest{RememberedAddress: settings.Device.NativeBluetoothAddress, ConnectTimeout: ble.DefaultConnectTimeout, ScanTimeout: 8 * time.Second})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if !s.nativeBluetooth.owner.Status().Simulated {
		if _, _, err := s.store.UpdateContext(ctx, func(settings config.Settings) (config.Settings, error) {
			settings.Device.NativeBluetoothAddress = device.Address
			return settings, nil
		}); err != nil {
			s.logger.Warn("could not remember native Bluetooth device")
		}
	}
	writeJSON(w, http.StatusOK, s.nativeBluetoothSnapshot())
}

func (s *Server) handleNativeBluetoothDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireNativeBluetoothHost(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	err := s.stopForHandyDisconnect(ctx, "native_bluetooth_disconnected")
	err = errors.Join(err, s.nativeBluetooth.owner.Disconnect(ctx))
	if err != nil {
		writeError(w, http.StatusBadGateway, errors.New("bluetooth disconnected; the device Stop could not be confirmed"))
		return
	}
	writeJSON(w, http.StatusOK, s.nativeBluetoothSnapshot())
}

func (s *Server) handleNativeBluetoothForget(w http.ResponseWriter, r *http.Request) {
	if !s.requireNativeBluetoothHost(w, r) {
		return
	}
	s.settingsLifecycleMu.Lock()
	defer s.settingsLifecycleMu.Unlock()
	status := s.nativeBluetooth.owner.Status()
	if status.Connected || status.Connecting {
		writeError(w, http.StatusConflict, errors.New("disconnect native Bluetooth before forgetting this device"))
		return
	}
	if _, _, err := s.store.UpdateContext(r.Context(), func(settings config.Settings) (config.Settings, error) {
		settings.Device.NativeBluetoothAddress = ""
		return settings, nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.nativeBluetoothSnapshot())
}

func (s *Server) configureDeviceWorkers(app Runtime, settings config.Settings, logger *slog.Logger) {
	s.configureCoyote(app, settings, logger)
	s.configureNativeBluetooth(app, logger)
}
