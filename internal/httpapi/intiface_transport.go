package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type intifaceRuntime struct {
	opMu        sync.Mutex
	mu          sync.Mutex
	owner       *transport.Intiface
	httpClient  *http.Client
	diagnostics transport.TransportDiagnostics
}

type intifaceSnapshot struct {
	DispatchOwner string                         `json:"dispatch_owner"`
	Address       string                         `json:"address"`
	Status        transport.IntifaceStatus       `json:"status"`
	Diagnostics   transport.TransportDiagnostics `json:"diagnostics"`
}

func newIntifaceRuntime(runtime Runtime) intifaceRuntime {
	return intifaceRuntime{
		httpClient: runtime.IntifaceHTTPClient,
		diagnostics: transport.TransportDiagnostics{
			Name:          "intiface_buttplug_v3",
			PlaybackState: "idle",
		},
	}
}

func (s *Server) handleIntifaceStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.intifaceSnapshot())
}

func (s *Server) handleIntifaceDiagnostics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.intifaceSnapshot().Diagnostics)
}

func (s *Server) handleIntifaceConnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerIntiface {
		writeError(w, http.StatusConflict, errors.New("select and save the Intiface dispatch owner before connecting"))
		return
	}
	s.intiface.mu.Lock()
	existing := s.intiface.owner
	alreadyConnected := existing != nil && existing.Status().Connected
	s.intiface.mu.Unlock()
	if alreadyConnected {
		writeError(w, http.StatusConflict, errors.New("intiface is already connected"))
		return
	}
	if existing != nil {
		s.closeIntifaceSession()
	}

	owner, err := transport.NewIntiface(transport.IntifaceOptions{
		Address:    settings.Device.IntifaceServerAddress,
		HTTPClient: s.intiface.httpClient,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	err = owner.Connect(ctx)
	cancel()
	if err != nil {
		_ = owner.Close()
		s.logger.Warn("Intiface connection failed", "error", err)
		writeError(w, http.StatusBadGateway, errors.New("intiface Central is not reachable; confirm it is running and the saved server address is correct"))
		return
	}

	s.intiface.mu.Lock()
	previous := s.intiface.owner
	s.intiface.owner = owner
	s.intiface.diagnostics = owner.Diagnostics()
	s.intiface.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	writeJSON(w, http.StatusOK, s.intifaceSnapshot())
}

func (s *Server) handleIntifaceDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	s.stopAndClearMotionEngine(r.Context(), "intiface_disconnected")
	s.closeIntifaceSession()
	writeJSON(w, http.StatusOK, s.intifaceSnapshot())
}

func (s *Server) handleIntifaceStartScan(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	owner, err := s.currentIntiface()
	if err == nil {
		err = owner.StartScanning(r.Context())
	}
	s.writeIntifaceResult(w, err)
}

func (s *Server) handleIntifaceStopScan(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	owner, err := s.currentIntiface()
	if err == nil {
		err = owner.StopScanning(r.Context())
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
	s.stopAndClearMotionEngine(r.Context(), "intiface_selection_changed")
	owner, err := s.currentIntiface()
	if err == nil {
		err = owner.SelectDevice(body.DeviceIndex, body.ActuatorIndex)
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

func (s *Server) currentIntiface() (*transport.Intiface, error) {
	s.intiface.mu.Lock()
	defer s.intiface.mu.Unlock()
	if s.intiface.owner == nil || !s.intiface.owner.Status().Connected {
		return nil, errors.New("intiface is not connected")
	}
	return s.intiface.owner, nil
}

func (s *Server) intifaceSnapshot() intifaceSnapshot {
	settings, _ := s.store.Snapshot()
	s.intiface.mu.Lock()
	defer s.intiface.mu.Unlock()
	snapshot := intifaceSnapshot{
		DispatchOwner: settings.Device.HSPDispatchOwner,
		Address:       settings.Device.IntifaceServerAddress,
		Status: transport.IntifaceStatus{
			PlaybackState: "idle",
			Devices:       []transport.IntifaceDevice{},
		},
		Diagnostics: s.intiface.diagnostics,
	}
	if s.intiface.owner != nil {
		snapshot.Status = s.intiface.owner.Status()
		snapshot.Diagnostics = s.intiface.owner.Diagnostics()
	}
	return snapshot
}

func (s *Server) closeIntiface() {
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	s.closeIntifaceSession()
}

func (s *Server) closeIntifaceSession() {
	s.intiface.mu.Lock()
	owner := s.intiface.owner
	s.intiface.owner = nil
	s.intiface.mu.Unlock()
	if owner != nil {
		_ = owner.Close()
		s.intiface.mu.Lock()
		s.intiface.diagnostics = owner.Diagnostics()
		s.intiface.mu.Unlock()
	}
}
