package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (s *Server) connectNamedHandyIntiface(ctx context.Context, id string) (transport.Transport, error) {
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	hub, err := s.currentIntifaceHub()
	if err != nil {
		settings, _ := s.store.Snapshot()
		if err = s.openIntifaceHub(ctx, settings.Device.IntifaceServerAddress); err != nil {
			return nil, err
		}
		hub, err = s.currentIntifaceHub()
		if err != nil {
			return nil, err
		}
	}
	owner := hub.LinearFor(id)
	s.intiface.mu.Lock()
	s.intiface.leases[id] = true
	s.intiface.mu.Unlock()
	return owner, nil
}

func (s *Server) releaseNamedHandyIntiface(ctx context.Context, id string) error {
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	s.intiface.mu.Lock()
	hub := s.intiface.hub
	delete(s.intiface.leases, id)
	remaining := len(s.intiface.leases)
	s.intiface.mu.Unlock()
	if hub == nil {
		return nil
	}
	err := hub.RemoveLinear(ctx, id)
	if remaining == 0 {
		s.closeIntifaceSession()
	}
	return err
}

func (s *Server) handleSelectHandy(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.settingsLifecycleMu.Lock()
	defer s.settingsLifecycleMu.Unlock()
	id := r.PathValue("id")
	settings, _ := s.store.Snapshot()
	c, ok := settings.Device.HandyConnection(id)
	if !ok || c.Method != config.DispatchOwnerIntiface {
		writeError(w, http.StatusConflict, errors.New("select Intiface for this Handy first"))
		return
	}
	var body struct {
		DeviceIndex   uint32 `json:"device_index"`
		ActuatorIndex uint32 `json:"actuator_index"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entry := s.handyEntry(id)
	entry.opMu.Lock()
	defer entry.opMu.Unlock()
	owner, _ := entry.current()
	linear, ok := owner.(*transport.Intiface)
	if !ok {
		writeError(w, http.StatusConflict, errors.New("connect this Handy through Intiface first"))
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
	defer cancel()
	if settings.Device.Plays(id) {
		finish, _ := s.beginGlobalStop("handy_selection_changed", r.Context())
		defer finish()
		if err := s.stopAndClearMotionEngine(ctx, "handy_selection_changed"); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	s.intiface.opMu.Lock()
	defer s.intiface.opMu.Unlock()
	if err := linear.Select(ctx, body.DeviceIndex, body.ActuatorIndex); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	entry.mu.Lock()
	entry.generation++
	entry.mu.Unlock()
	if hub, err := s.currentIntifaceHub(); err == nil && hub.Status().Scanning {
		_ = hub.StopScanning(ctx)
	}
	writeJSON(w, http.StatusOK, s.handyConnectionSnapshot(id))
}
