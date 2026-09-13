package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	controllerHeaderName       = "X-MagicHandy-Client-ID"
	controllerGenerationHeader = "X-MagicHandy-Control-Generation"
	controllerEpochHeader      = "X-MagicHandy-Control-Epoch"
	controllerLeaseTTL         = 15 * time.Second
)

var (
	errControllerTakeoverInProgress = errors.New("another controller takeover is already in progress")
	errControllerTakeoverNotPending = errors.New("controller takeover is no longer pending")
)

type controllerSnapshot struct {
	Revision              uint64 `json:"revision"`
	CommandTicket         string `json:"command_ticket,omitempty"`
	CommandTicketMillis   int64  `json:"command_ticket_ms,omitempty"`
	CommandSequence       uint64 `json:"command_sequence"`
	Epoch                 string `json:"epoch"`
	Generation            uint64 `json:"generation"`
	HeartbeatRequired     bool   `json:"heartbeat_required"`
	ClientID              string `json:"client_id,omitempty"`
	Active                bool   `json:"active"`
	ReadOnly              bool   `json:"read_only"`
	Reason                string `json:"reason,omitempty"`
	ActiveClientID        string `json:"active_client_id,omitempty"`
	ActiveClientAgeMillis int64  `json:"active_client_age_ms,omitempty"`
	LeaseExpiresInMillis  int64  `json:"lease_expires_in_ms,omitempty"`
	TakeoverInProgress    bool   `json:"takeover_in_progress,omitempty"`
}

type controllerTakeoverResponse struct {
	Controller    controllerSnapshot `json:"controller"`
	Changed       bool               `json:"changed"`
	StopConfirmed bool               `json:"stop_confirmed"`
	StopSequence  uint64             `json:"stop_sequence"`
	Warning       string             `json:"warning,omitempty"`
}

func (s *Server) controllerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/controller", s.handleControllerState)
	mux.HandleFunc("POST /api/controller/heartbeat", s.handleControllerHeartbeat)
	mux.HandleFunc("POST /api/controller/takeover", s.handleControllerTakeover)
	mux.HandleFunc("GET /api/controller/commands/{id}", s.handleCommandReceipt)
}

func (s *Server) handleControllerState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.controllerState(r))
}

func (s *Server) handleControllerTakeover(w http.ResponseWriter, r *http.Request) {
	if !s.currentControllerEpoch(r) {
		writeError(w, http.StatusConflict, errors.New("server restarted; refresh before taking control"))
		return
	}
	if s.quiescing.Load() {
		writeError(w, http.StatusServiceUnavailable, errServerQuiescing)
		return
	}
	clientID := cleanControllerClientID(r.Header.Get(controllerHeaderName))
	if clientID == "" {
		writeError(w, http.StatusBadRequest, errors.New("controller takeover requires a client id header"))
		return
	}

	actor := controllerActor(r, clientID)
	controller, started, err := s.controller.BeginTakeover(actor)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":      err.Error(),
			"controller": controller,
		})
		return
	}
	if !started {
		writeJSON(w, http.StatusOK, controllerTakeoverResponse{
			Controller:    s.commandSnapshot(r, controller),
			Changed:       false,
			StopConfirmed: true,
			StopSequence:  s.stopSequence.Load(),
		})
		return
	}

	completed := false
	defer func() {
		if !completed {
			s.controller.CancelTakeover(actor)
		}
	}()

	_, stopErr := s.emergencyStop(r.Context(), "controller_takeover")
	if r.Context().Err() != nil {
		writeError(w, http.StatusConflict, errors.New("controller takeover was canceled"))
		return
	}
	controller, err = s.controller.CompleteTakeover(actor)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	completed = true

	response := controllerTakeoverResponse{
		Controller:    s.commandSnapshot(r, controller),
		Changed:       true,
		StopConfirmed: stopErr == nil,
		StopSequence:  s.stopSequence.Load(),
	}
	if stopErr != nil {
		response.Warning = "Control transferred after local Stop, but physical Stop could not be confirmed: " +
			s.safeMotionErrorMessage(stopErr)
	}
	s.logger.Info("controller ownership transferred",
		"stop_confirmed", response.StopConfirmed,
	)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) controllerState(r *http.Request) controllerSnapshot {
	snapshot := s.controller.Observe(controllerActor(r, clientIDFromRequest(r)))
	if !s.capabilities(r).Control {
		snapshot.Active, snapshot.ReadOnly = false, true
		snapshot.Reason = "this account is an observer; ask the administrator for a control permission"
	}
	return s.commandSnapshot(r, snapshot)
}

func (s *Server) commandSnapshot(r *http.Request, snapshot controllerSnapshot) controllerSnapshot {
	if snapshot.Active && snapshot.HeartbeatRequired {
		snapshot.CommandSequence = s.commands.lastSequence(commandScope{actor: controllerActor(r, clientIDFromRequest(r)), generation: snapshot.Generation})
	}
	return snapshot
}

func (s *Server) requireController(w http.ResponseWriter, r *http.Request) bool {
	if !s.capabilities(r).Control {
		writeError(w, http.StatusForbidden, errors.New("this account does not have permission to control motion"))
		return false
	}
	if s.quiescing.Load() {
		writeError(w, http.StatusServiceUnavailable, errServerQuiescing)
		return false
	}
	snapshot, _ := s.controller.Authority(controllerActor(r, cleanControllerClientID(r.Header.Get(controllerHeaderName))))
	generationOK := true
	if snapshot.HeartbeatRequired {
		generation, err := strconv.ParseUint(r.Header.Get(controllerGenerationHeader), 10, 64)
		generationOK = err == nil && generation == snapshot.Generation && s.currentControllerEpoch(r)
	}
	if snapshot.Active && generationOK && r.Context().Err() == nil {
		if binding, ok := r.Context().Value(controllerRequestBindingKey{}).(*controllerRequestBinding); ok &&
			!binding.bind(&s.controller, controllerActor(r, clientIDFromRequest(r))) {
			writeError(w, http.StatusConflict, errors.New("controller changed before command admission"))
			return false
		}
		return s.admitControlCommand(w, r, snapshot)
	}
	message := "this client is read-only; the active controller owns device commands"
	if snapshot.Active && !generationOK {
		message = "controller state changed; refresh before sending another command"
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":      message,
		"controller": snapshot,
	})
	return false
}

func controllerActor(r *http.Request, clientID string) controllerIdentity {
	actor := controllerIdentity{clientID: clientID}
	if session, ok := authenticatedSession(r); ok {
		actor.sessionKey = session.session.Key
		if grant := session.session.ControlGrant; grant != nil {
			actor.grantID, actor.grantExpires = grant.ID, grant.ExpiresAt
		}
	}
	return actor
}

func (s *Server) currentControllerEpoch(r *http.Request) bool {
	_, authenticated := authenticatedSession(r)
	return !authenticated || r.Header.Get(controllerEpochHeader) == s.controller.epoch
}

func (s *Server) handleControllerHeartbeat(w http.ResponseWriter, r *http.Request) {
	if s.quiescing.Load() {
		writeError(w, http.StatusServiceUnavailable, errServerQuiescing)
		return
	}
	if !s.capabilities(r).Control {
		writeJSON(w, http.StatusOK, s.controllerState(r))
		return
	}
	actor := controllerActor(r, cleanControllerClientID(r.Header.Get(controllerHeaderName)))
	if actor.clientID == "" {
		writeError(w, http.StatusBadRequest, errors.New("controller heartbeat requires a client id header"))
		return
	}
	generation, err := strconv.ParseUint(r.Header.Get(controllerGenerationHeader), 10, 64)
	if actor.sessionKey != "" && (err != nil || !s.currentControllerEpoch(r)) {
		writeJSON(w, http.StatusOK, s.controllerState(r))
		return
	}
	writeJSON(w, http.StatusOK, s.commandSnapshot(r, s.controller.Heartbeat(actor, generation)))
}

func (s *Server) bootstrapController(r *http.Request, sessionKey string) {
	actor := controllerIdentity{clientID: cleanControllerClientID(r.Header.Get(controllerHeaderName)), sessionKey: sessionKey}
	if actor.clientID == "" {
		if s.controller.BeginLocalLoss() {
			s.stopLostController("account_protection_enabled")
		}
		return
	}
	_, started, err := s.controller.BeginTakeover(actor)
	if err != nil || !started {
		return
	}
	defer s.controller.CancelTakeover(actor)
	_, _ = s.emergencyStop(r.Context(), "account_protection_enabled")
	if r.Context().Err() == nil {
		_, _ = s.controller.CompleteTakeover(actor)
	}
}

func clientIDFromRequest(r *http.Request) string {
	clientID := strings.TrimSpace(r.Header.Get(controllerHeaderName))
	if clientID == "" {
		clientID = strings.TrimSpace(r.URL.Query().Get("client_id"))
	}
	return cleanControllerClientID(clientID)
}

func cleanControllerClientID(clientID string) string {
	clientID = strings.TrimSpace(clientID)
	if len(clientID) > 120 {
		clientID = clientID[:120]
	}
	clientID = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return -1
		}
	}, clientID)
	return clientID
}
