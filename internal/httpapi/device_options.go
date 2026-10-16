package httpapi

import (
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// handlePatchDeviceOptions saves the device options that Settings > Devices
// changes one at a time. Each change takes the same runtime transition as a
// full settings save: a new Intiface address ends the shared session, a new
// failure policy or Cloud application ID stops the current run, and Coyote
// and vibrator settings reach their owners at once.
func (s *Server) handlePatchDeviceOptions(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var patch config.DeviceOptionsPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var patchErr error
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		var next config.Settings
		next, patchErr = current.ApplyDeviceOptionsPatch(patch)
		return next, patchErr
	})
	switch {
	case patchErr != nil:
		writeError(w, http.StatusBadRequest, patchErr)
		return
	case saveErr != nil:
		writeError(w, http.StatusInternalServerError, errors.New("device settings could not be saved"))
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

// handlePutServerPort saves the port the local server listens on. The
// running server keeps its port; the new one applies at the next start.
func (s *Server) handlePutServerPort(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Port int `json:"port"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Port < 1 || body.Port > 65535 {
		writeError(w, http.StatusBadRequest, errors.New("server port must be between 1 and 65535"))
		return
	}
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		current.Server.Port = body.Port
		return current, nil
	})
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, errors.New("the server port could not be saved"))
		return
	}
	payload := map[string]any{"settings": s.clientSettings(r, saved.Public())}
	status := http.StatusOK
	if runtimeErr != nil {
		status = http.StatusBadGateway
		payload["error"] = "settings were saved, but the active runtime could not apply them"
	}
	writeJSON(w, status, payload)
}
