package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/remote"
)

const (
	remoteKeepaliveInterval = 15 * time.Second
	// Remote state is re-read on this cadence even without a change, so a
	// desktop that stops reporting is shown as gone once its presence expires.
	remoteStateRecheck = 5 * time.Second
	maxRemoteOutcomes  = 64
)

// A phone (or another window) asks the desktop tab that holds control to act:
// play, pause or seek its video, open another one, or send a chat message. The
// desktop carries each command out with its own controls and its own controller
// authority, so every device effect still passes the usual admission, Stop
// fencing and shared engine. Only the same account may send commands to a
// desktop. See internal/remote and docs/decisions/0032-video-curation-and-remote.md.
func (s *Server) remoteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/remote/presence", s.handleRemotePresence)
	mux.HandleFunc("DELETE /api/remote/presence", s.handleRemoteWithdraw)
	s.remoteClientRoutes(mux)
	mux.HandleFunc("POST /api/remote/commands/{id}/claim", s.handleRemoteClaim)
}

func remoteIdentity(r *http.Request, clientID string) remote.Identity {
	identity := remote.Identity{ClientID: clientID}
	if session, ok := authenticatedSession(r); ok {
		identity.AccountID = session.session.Account.ID
		identity.SessionKey = session.session.Key
		identity.DesktopAccountID = session.session.RemoteDesktopAccount()
		if session.session.ControlGrant != nil {
			identity.GrantID = session.session.ControlGrant.ID
		}
	}
	return identity
}

// The remote is a control feature: an observer cannot send commands, so it
// has no reason to see what the desktop is showing either.
func (s *Server) requireRemoteControl(w http.ResponseWriter, r *http.Request) bool {
	if s.capabilities(r).RemoteControl {
		return true
	}
	writeError(w, http.StatusForbidden, errors.New("this account does not have permission to control motion"))
	return false
}

// remoteExecutor admits the tab that holds control, at its current generation.
// A presence report is not a device command: it carries no command ticket, so
// it never waits behind live control and adds nothing to the audit log.
func (s *Server) remoteExecutor(w http.ResponseWriter, r *http.Request) (remote.Identity, bool) {
	if !s.requireRemoteControl(w, r) {
		return remote.Identity{}, false
	}
	if s.quiescing.Load() {
		writeError(w, http.StatusServiceUnavailable, errServerQuiescing)
		return remote.Identity{}, false
	}
	clientID := clientIDFromRequest(r)
	snapshot, _ := s.controller.Authority(controllerActor(r, clientID))
	current := true
	if snapshot.HeartbeatRequired {
		generation, err := strconv.ParseUint(r.Header.Get(controllerGenerationHeader), 10, 64)
		current = err == nil && generation == snapshot.Generation && s.currentControllerEpoch(r)
	}
	if clientID == "" || !snapshot.Active || !current {
		writeError(w, http.StatusConflict, errors.New("this tab does not hold control"))
		return remote.Identity{}, false
	}
	return remoteIdentity(r, clientID), true
}

func (s *Server) remoteExecutorActive(r *http.Request, clientID string) bool {
	return s.controller.Holds(controllerActor(r, clientID))
}

// handleRemotePresence records what the controller tab is showing and the
// outcomes of the commands it carried out.
func (s *Server) handleRemotePresence(w http.ResponseWriter, r *http.Request) {
	executor, ok := s.remoteExecutor(w, r)
	if !ok {
		return
	}
	var presence remote.Presence
	if err := decodeJSON(r, &presence); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(presence.Outcomes) > maxRemoteOutcomes {
		writeError(w, http.StatusBadRequest, errors.New("too many command outcomes in one report"))
		return
	}
	// Reading the body may have waited across logout or a controller handoff.
	if r.Context().Err() != nil || !s.remoteExecutorActive(r, executor.ClientID) || !s.remoteSenderActive(r, executor) {
		writeError(w, http.StatusConflict, remote.ErrCommandUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remote": s.remote.Report(executor, presence)})
}

// handleRemoteWithdraw ends a tab's presence when it closes, hides or loses
// control. By then it may no longer hold control, so only its own session and
// tab identity are required.
func (s *Server) handleRemoteWithdraw(w http.ResponseWriter, r *http.Request) {
	s.remote.Withdraw(remoteIdentity(r, clientIDFromRequest(r)), "The desktop stopped taking commands.")
	writeJSON(w, http.StatusOK, map[string]any{"status": "withdrawn"})
}

func (s *Server) handleRemoteState(w http.ResponseWriter, r *http.Request) {
	if !s.requireRemoteControl(w, r) {
		return
	}
	state := s.remoteState(r)
	writeJSON(w, http.StatusOK, map[string]any{"remote": state})
}

// handleRemoteCommand queues a command. It needs control permission but not
// the controller lease: the phone asks, and the desktop that holds the lease
// decides and acts. Sending is user activity, so it renews the phone's login.
func (s *Server) handleRemoteCommand(w http.ResponseWriter, r *http.Request) {
	if !s.requireRemoteControl(w, r) {
		return
	}
	sequence, ok := s.remoteStopSequence(w, r)
	if !ok {
		return
	}
	var command remote.Command
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sender := remoteIdentity(r, clientIDFromRequest(r))
	if !s.remoteSenderActive(r, sender) || s.stopSequence.Load() != sequence {
		writeError(w, http.StatusConflict, remote.ErrCommandUnavailable)
		return
	}
	command.StopSequence = sequence
	sent, err := s.remote.Send(sender, command)
	if err == nil {
		if s.stopSequence.Load() != sequence {
			s.remote.Cancel(sent.ID, "Emergency Stop cleared the command.")
			writeError(w, http.StatusConflict, remote.ErrCommandUnavailable)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"command": sent})
		return
	}
	// The code lets the phone word the failure in its own language.
	status, code := http.StatusConflict, "no_desktop"
	switch {
	case errors.Is(err, remote.ErrInvalidCommand):
		status, code = http.StatusBadRequest, "invalid"
	case errors.Is(err, remote.ErrOtherAccount):
		status, code = http.StatusForbidden, "other_account"
	case errors.Is(err, remote.ErrBusy):
		status, code = http.StatusTooManyRequests, "busy"
	case errors.Is(err, remote.ErrTargetUnavailable):
		code = "target_unavailable"
	}
	writeJSON(w, status, map[string]string{"error": err.Error(), "code": code})
}

func (s *Server) remoteStopSequence(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	if strings.TrimSpace(r.Header.Get(stopSequenceHeader)) == "" {
		writeError(w, http.StatusConflict, errors.New("remote commands require the current Emergency Stop sequence"))
		return 0, false
	}
	sequence, err := s.requestStopSequence(r)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return 0, false
	}
	return sequence, true
}

func (s *Server) remoteSenderActive(r *http.Request, sender remote.Identity) bool {
	if r.Context().Err() != nil {
		return false
	}
	if sender.SessionKey == "" {
		return !s.auth.authenticationRequired() && sender.AccountID == ""
	}
	session, err := s.accounts.CheckSession(r.Context(), sender.SessionKey)
	grantID := ""
	if session.ControlGrant != nil {
		grantID = session.ControlGrant.ID
	}
	return err == nil && session.Account.ID == sender.AccountID && session.RemoteDesktopAccount() == sender.DesktopAccount() && grantID == sender.GrantID && session.CanUseRemote(time.Now())
}

// Revalidate queued intent immediately before execution. A buffered SSE event
// must not survive expiry, Stop, logout, a changed desktop or a changed video.
// The returned Stop sequence must also fence the actual player/chat request.
func (s *Server) handleRemoteClaim(w http.ResponseWriter, r *http.Request) {
	executor, ok := s.remoteExecutor(w, r)
	if !ok {
		return
	}
	sequence, ok := s.remoteStopSequence(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	sender, found := s.remote.CommandSender(executor, id)
	if !found || !s.remoteSenderActive(r, sender) {
		if found {
			s.remote.Cancel(id, "The command is no longer available. Send it again.")
		}
		writeError(w, http.StatusConflict, remote.ErrCommandUnavailable)
		return
	}
	command, remaining, err := s.remote.Claim(executor, id)
	if err != nil || command.StopSequence != sequence || s.stopSequence.Load() != sequence {
		if err == nil {
			s.remote.Cancel(id, "Emergency Stop cleared the command.")
		}
		writeError(w, http.StatusConflict, remote.ErrCommandUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"command": command, "remaining_ms": remaining.Milliseconds()})
}

// deliverRemoteCommands forwards commands queued for this tab on its existing
// motion stream, so the desktop holds no extra connection beside its video,
// chat and speech streams. A tab that lost control gets nothing; its waiting
// commands are dropped and the phone is told why.
func (s *Server) deliverRemoteCommands(w http.ResponseWriter, r *http.Request, tab remote.Identity, after *uint64) bool {
	if s.remote == nil || tab.ClientID == "" {
		return true
	}
	commands, _ := s.remote.Commands(tab, *after)
	if len(commands) == 0 {
		return true
	}
	if !s.remoteExecutorActive(r, tab.ClientID) {
		s.remote.Withdraw(tab, "Control moved to another tab; send the command again.")
		return true
	}
	for _, command := range commands {
		if err := writeSSE(w, "remote_command", command); err != nil {
			return false
		}
		*after = command.Sequence
	}
	return true
}

// handleRemoteEvents streams the desktop's state to remotes whenever it
// changes, and re-checks it periodically so an expired desktop disappears.
func (s *Server) handleRemoteEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := w.(http.Flusher); !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming responses are unavailable"))
		return
	}
	if !s.requireRemoteControl(w, r) {
		return
	}
	viewer := remoteIdentity(r, clientIDFromRequest(r))
	setSSEHeaders(w)
	w.WriteHeader(http.StatusOK)
	recheck := time.NewTicker(remoteStateRecheck)
	defer recheck.Stop()
	keepalive := time.NewTicker(remoteKeepaliveInterval)
	defer keepalive.Stop()
	var sent remote.State
	first := true
	for {
		if !s.remoteSenderActive(r, viewer) {
			return
		}
		state, changed := s.remote.State(viewer)
		state.StopSequence, state.CanControl = s.stopSequence.Load(), true
		if first || state.Revision != sent.Revision || state.Connected != sent.Connected || state.StopSequence != sent.StopSequence {
			if err := writeSSE(w, "state", state); err != nil {
				return
			}
			sent, first = state, false
		}
		select {
		case <-s.lifecycleCtx.Done():
			return
		case <-r.Context().Done():
			return
		case <-changed:
		case <-recheck.C:
		case <-keepalive.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flush(w)
		}
	}
}

func flush(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
