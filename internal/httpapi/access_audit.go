package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

func (s *Server) auditRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/audit", s.handleAccessAudit)
	mux.HandleFunc("GET /api/audit/export", s.handleAccessAudit)
}

func (s *Server) handleAccessAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdministrator(w, r); !ok {
		return
	}
	before := int64(0)
	if value := r.URL.Query().Get("before"); value != "" {
		var err error
		before, err = strconv.ParseInt(value, 10, 64)
		if err != nil || before < 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid access history cursor"))
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := s.auditStore.Page(ctx, before)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("access history is unavailable"))
		return
	}
	data, err := json.Marshal(struct {
		audit.Page
		Writer audit.Status `json:"writer"`
	}{page, s.accessAudit.Status()})
	if err != nil || len(data) > audit.PageMaxBytes {
		writeError(w, http.StatusInternalServerError, errors.New("access history exceeds its response limit"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasSuffix(r.URL.Path, "/export") {
		w.Header().Set("Content-Disposition", `attachment; filename="magichandy-access-history.json"`)
	}
	if err := writeContextBytes(ctx, w, append(data, '\n')); err != nil {
		panic(http.ErrAbortHandler)
	}
}

func (s *Server) recordAccessEvent(ctx context.Context, event audit.Event) {
	if s.accessAudit == nil {
		return
	}
	if event.Actor.Type == "" {
		event.Actor = audit.ContextActor(ctx)
	}
	if event.Epoch == "" {
		event.Epoch = s.controller.epoch
	}
	if event.TraceSequence == 0 && s.traces != nil {
		event.TraceSequence = s.traces.LatestSequence()
	}
	s.accessAudit.Record(event)
}

func (s *Server) recordRejectedLogin(r *http.Request, err error) {
	if r.URL.Path != "/api/auth/login" {
		return
	}
	kind := audit.LoginFailed
	if errors.Is(err, errAuthenticationThrottled) {
		kind = audit.LoginThrottled
	}
	s.recordAccessEvent(r.Context(), audit.Event{Kind: kind, Outcome: "rejected", Actor: audit.Actor{Type: "public"}})
}

func auditCommandOperation(route string) string {
	switch {
	case strings.HasPrefix(route, "/api/chat/"):
		return "chat"
	case strings.HasPrefix(route, "/api/media/"):
		return "media"
	case strings.HasPrefix(route, "/api/modes/"):
		return "mode"
	case strings.HasPrefix(route, "/api/motion/"):
		return "motion"
	case strings.Contains(route, "/feedback"):
		return "feedback"
	case strings.Contains(route, "/preferences") || strings.Contains(route, "/input-preferences") || route == "/api/settings/llm-motion-mode":
		return "preferences"
	default:
		return "host"
	}
}

func auditStopOperation(reason string) string {
	switch reason {
	case "controller_heartbeat_expired":
		return "heartbeat_expired"
	case "controller_session_expired", "controller_session_revoked":
		return "session_ended"
	case "controller_permission_ended":
		return "permission_ended"
	case "controller_takeover", "account_protection_enabled":
		return "takeover"
	case "server_shutdown":
		return "shutdown"
	default:
		return "emergency"
	}
}

func (s *Server) recordCommandResult(r *http.Request, invocation *commandInvocation) {
	status := invocation.capture.status
	outcome := "success"
	if invocation.capture.interrupted || status == 0 {
		outcome = "unknown"
	} else if status >= 500 {
		outcome = "failed"
	} else if status >= 400 {
		outcome = "rejected"
	}
	receipt := invocation.receipt
	s.recordAccessEvent(r.Context(), audit.Event{Kind: audit.CommandFinished, Outcome: outcome,
		Operation: auditCommandOperation(r.URL.Path), Epoch: receipt.Epoch, Generation: receipt.Generation,
		Correlation: audit.CorrelateCommand(receipt.key.id), HTTPStatus: status})
}
