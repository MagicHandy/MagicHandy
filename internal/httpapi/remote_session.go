package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/audit"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/remote"
)

// errRemoteNoSession reports that no desktop of the phone's account holds control.
var errRemoteNoSession = errors.New("no desktop session of this account holds control")

// remoteKeepalive is what a phone learns from one keepalive.
type remoteKeepalive struct {
	Renewed bool `json:"renewed"`
	// LeaseExpiresInMillis is the renewed lease's remaining lifetime.
	LeaseExpiresInMillis int64 `json:"lease_expires_in_ms,omitempty"`
}

// remoteDesktopOwner returns the controller tab a phone may act for: the live
// controller of the phone's desktop account. A hidden desktop has withdrawn its
// presence, so the account comes from its last report, or from its login.
func (s *Server) remoteDesktopOwner(ctx context.Context, sender remote.Identity) (controllerIdentity, bool) {
	owner := s.controller.Owner()
	if owner.clientID == "" {
		return controllerIdentity{}, false
	}
	if owner.sessionKey == "" {
		// Unprotected local control has no account; only a local phone matches.
		return owner, sender.SessionKey == "" && !s.auth.authenticationRequired()
	}
	if sender.SessionKey == "" {
		return controllerIdentity{}, false
	}
	account, known := s.remote.ExecutorAccount(owner.sessionKey)
	if !known {
		session, err := s.accounts.CheckSession(ctx, owner.sessionKey)
		if err != nil {
			return controllerIdentity{}, false
		}
		account = session.Account.ID
	}
	return owner, account != "" && account == sender.DesktopAccount()
}

// handleRemoteKeepalive lets a phone in front keep its desktop's session
// alive while the desktop tab is hidden (ADR 0055). It renews only a live lease
// of the phone's desktop account, and it is passive for login idle time.
func (s *Server) handleRemoteKeepalive(w http.ResponseWriter, r *http.Request) {
	if !s.requireRemoteControl(w, r) {
		return
	}
	if s.quiescing.Load() {
		writeError(w, http.StatusServiceUnavailable, errServerQuiescing)
		return
	}
	sender := remoteIdentity(r, clientIDFromRequest(r))
	if !s.remoteSenderActive(r, sender) {
		writeError(w, http.StatusConflict, remote.ErrCommandUnavailable)
		return
	}
	result := remoteKeepalive{}
	if owner, ok := s.remoteDesktopOwner(r.Context(), sender); ok && s.controller.RemoteRenew(owner) {
		result = remoteKeepalive{Renewed: true, LeaseExpiresInMillis: s.controller.leaseTTL.Milliseconds()}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keepalive": result})
}

// remoteSession describes the phone's desktop session: whether it is alive,
// and the motion and Edge state the phone's Edge button needs.
func (s *Server) remoteSession(ctx context.Context, viewer remote.Identity) *remote.Session {
	owner, ok := s.remoteDesktopOwner(ctx, viewer)
	if !ok || !s.controller.OwnerLive(owner) {
		return &remote.Session{}
	}
	session := &remote.Session{Alive: true, StopOnExpiry: owner.sessionKey != ""}
	if engine := s.currentMotionEngine(); engine != nil {
		snapshot := engine.Snapshot()
		session.Running, session.Paused = snapshot.Running, snapshot.Paused
		// The same admission as the desktop's Edge button.
		session.EdgeAllowed = snapshot.Running && !snapshot.Paused && snapshot.Target.MediaID == "" && snapshot.Target.ProgramID == ""
	}
	if s.modes != nil {
		if edge := s.modes.Status().EdgeHold; edge != nil {
			session.Edge = &remote.EdgeHold{Active: edge.Active, Phase: edge.Phase, Automatic: edge.Automatic,
				ResumeInSeconds: edge.ResumeInSeconds, Message: edge.Message}
		}
	}
	return session
}

func (s *Server) handleRemoteEdge(w http.ResponseWriter, r *http.Request) {
	s.remoteEdge(w, r, false)
}

func (s *Server) handleRemoteEdgeResume(w http.ResponseWriter, r *http.Request) {
	s.remoteEdge(w, r, true)
}

// remoteEdge applies Edge, or returns from it, for the phone's desktop session
// (ADR 0055). Like Stop, a gentle hold must not wait for a desktop tab that may
// be hidden or asleep, so the server applies it directly. It needs control
// permission, the same desktop account as a live controller and the current
// Emergency Stop sequence; the shared engine and its plan guards do the rest.
func (s *Server) remoteEdge(w http.ResponseWriter, r *http.Request, returning bool) {
	if !s.requireRemoteControl(w, r) {
		return
	}
	if s.quiescing.Load() {
		writeError(w, http.StatusServiceUnavailable, errServerQuiescing)
		return
	}
	sequence, ok := s.remoteStopSequence(w, r)
	if !ok {
		return
	}
	sender := remoteIdentity(r, clientIDFromRequest(r))
	owner, admitted := s.remoteDesktopOwner(r.Context(), sender)
	if !admitted {
		writeError(w, http.StatusConflict, errRemoteNoSession)
		return
	}
	// The phone acts for a different tab: ordinary session middleware cannot
	// bind it to that tab's authority. Bind explicitly before revalidation so
	// Stop or handoff synchronously cancels a press waiting in mode application.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	detach := s.controller.bindRequest(owner, cancel)
	if detach == nil {
		writeError(w, http.StatusConflict, errRemoteNoSession)
		return
	}
	defer detach()
	r = r.WithContext(ctx)
	if !s.remoteSenderActive(r, sender) || s.stopSequence.Load() != sequence {
		writeError(w, http.StatusConflict, errRemoteNoSession)
		return
	}
	var err error
	if returning {
		_, err = s.resumeEdge(r.Context())
	} else {
		_, err = s.activateEdge(r.Context())
	}
	outcome, status := "success", http.StatusOK
	if err != nil {
		outcome, status = "rejected", http.StatusConflict
	}
	s.recordAccessEvent(r.Context(), audit.Event{Kind: audit.CommandFinished, Outcome: outcome, Operation: "motion", HTTPStatus: status})
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": s.remoteSession(r.Context(), sender)})
}

// activateEdge applies the gentle hold for any admitted caller. Chat and
// planning in flight are canceled without the Stop path; their own contexts and
// plan guards reject late work.
func (s *Server) activateEdge(ctx context.Context) (modes.Status, error) {
	s.chatWorkspace.CancelTurns()
	s.motionReview.invalidate()
	s.cloudRequests.invalidate()
	s.autopilotHold.release()
	return s.modes.ActivateEdge(ctx)
}

func (s *Server) resumeEdge(ctx context.Context) (modes.Status, error) {
	return s.modes.ResumeEdge(ctx)
}
