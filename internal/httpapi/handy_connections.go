package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func (s *Server) handyConnectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/settings/handys", s.handleAddHandy)
	mux.HandleFunc("PATCH /api/settings/handys/{id}", s.handlePatchHandy)
	mux.HandleFunc("DELETE /api/settings/handys/{id}", s.handleRemoveHandy)
	mux.HandleFunc("GET /api/transport/handys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"connections": s.handyConnectionSnapshots()})
	})
	mux.HandleFunc("POST /api/transport/handys/{id}/connect", s.handleConnectHandy)
	mux.HandleFunc("POST /api/transport/handys/{id}/disconnect", s.handleDisconnectHandy)
	mux.HandleFunc("POST /api/transport/handys/{id}/discover", s.handleDiscoverHandy)
	mux.HandleFunc("POST /api/transport/handys/{id}/select", s.handleSelectHandy)
	mux.HandleFunc("POST /api/transport/handys/{id}/forget", s.handleForgetHandy)
}

func (s *Server) requireHandyHost(w http.ResponseWriter, r *http.Request) bool {
	if !s.requireController(w, r) {
		return false
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("only the host can configure Handy connections"))
		return false
	}
	return true
}

func newHandyID() string {
	var id [6]byte
	_, _ = rand.Read(id[:])
	return "handy-" + hex.EncodeToString(id[:])
}

func (s *Server) handleAddHandy(w http.ResponseWriter, r *http.Request) {
	if !s.requireHandyHost(w, r) {
		return
	}
	var patch config.HandyConnectionPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := newHandyID()
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		return current.AddHandyConnection(id, patch)
	})
	if saveErr != nil {
		writeError(w, http.StatusBadRequest, saveErr)
		return
	}
	if runtimeErr != nil {
		writeError(w, http.StatusBadGateway, errors.New("the Handy was saved, but the device runtime could not be updated"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.clientSettings(r, saved.Public()), "device_id": id})
}

func (s *Server) handlePatchHandy(w http.ResponseWriter, r *http.Request) {
	if !s.requireHandyHost(w, r) {
		return
	}
	var patch config.HandyConnectionPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.writeImmediateSettings(w, r, func(current config.Settings) (config.Settings, error) {
		return current.PatchHandyConnection(r.PathValue("id"), patch)
	})
}

func (s *Server) handleRemoveHandy(w http.ResponseWriter, r *http.Request) {
	if !s.requireHandyHost(w, r) {
		return
	}
	s.writeImmediateSettings(w, r, func(current config.Settings) (config.Settings, error) {
		return current.RemoveHandyConnection(r.PathValue("id"))
	})
}
