package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const (
	commandIDHeader       = "X-MagicHandy-Command-ID"
	commandSequenceHeader = "X-MagicHandy-Command-Sequence"
	commandTicketHeader   = "X-MagicHandy-Command-Ticket"
)

type commandInvocationKey struct{}
type commandInvocation struct {
	scope   commandScope
	receipt *commandReceipt
	body    *commandBody
	capture *commandResponse
	release func()
}

// Install the lifecycle before dispatch but admit only when a route actually
// requires control. Self-service account actions and Stop need no ticket.
func (s *Server) trackCommandDelivery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedSession(r); !ok || readRequest(r) || isStopDelivery(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		invocation := &commandInvocation{body: newCommandBody(r), capture: &commandResponse{ResponseWriter: w}}
		r.Body = invocation.body
		completed := false
		defer func() {
			if invocation.receipt != nil {
				invocation.capture.interrupted = !completed
				s.commands.finish(invocation.receipt, invocation.capture, invocation.body)
			}
			if invocation.release != nil {
				invocation.release()
			}
		}()
		next.ServeHTTP(invocation.capture, r.WithContext(context.WithValue(r.Context(), commandInvocationKey{}, invocation)))
		completed = true
	})
}

func isStopDelivery(route string) bool {
	switch route {
	case "/api/motion/stop", "/api/transport/cloud/stop", "/api/transport/bluetooth/stop":
		return true
	default:
		return false
	}
}

func serialControlRequest(route string) bool {
	// Inference and native dialogs retain their existing cancellable lifetimes;
	// they must not retain the lane used for live motion and setting updates.
	if motionCommandRoute(route) {
		return !deferredMotionCommand(route)
	}
	if route == "/api/voice/preferences" || route == "/api/voice/input-preferences" {
		return true
	}
	return route != "/api/host/path-picker" && route != "/api/motion/lab/proposal" &&
		!strings.HasPrefix(route, "/api/llm/") && !strings.HasPrefix(route, "/api/voice/") && !strings.HasPrefix(route, "/api/labs/")
}

func (s *Server) admitControlCommand(w http.ResponseWriter, r *http.Request, snapshot controllerSnapshot) bool {
	invocation, tracked := r.Context().Value(commandInvocationKey{}).(*commandInvocation)
	if !tracked || invocation.receipt != nil {
		return true
	}
	actor := controllerActor(r, cleanControllerClientID(r.Header.Get(controllerHeaderName)))
	if len(r.URL.RequestURI()) > 2048 {
		writeError(w, http.StatusRequestURITooLong, errors.New("control command URL exceeds 2048 bytes"))
		return false
	}
	id := strings.TrimSpace(r.Header.Get(commandIDHeader))
	sequence, err := strconv.ParseUint(r.Header.Get(commandSequenceHeader), 10, 64)
	if err != nil || sequence == 0 || sequence > (1<<53)-1 || len(id) < 8 || len(id) > 120 || cleanControllerClientID(id) != id {
		writeError(w, http.StatusBadRequest, errors.New("control commands require a unique command ID and positive sequence"))
		return false
	}
	key := commandReceiptKey{session: actor.sessionKey, client: actor.clientID, id: id}
	if receipt := s.commands.lookup(key); receipt != nil {
		if receipt.Epoch != snapshot.Epoch || receipt.Generation != snapshot.Generation {
			writeError(w, http.StatusConflict, errors.New("command belongs to an earlier control generation; inspect its receipt and current state"))
			return false
		}
		return s.replayControlCommand(w, r, invocation, receipt)
	}
	if serialControlRequest(r.URL.Path) || deferredMotionCommand(r.URL.Path) {
		invocation.release, err = s.commands.acquire(r.Context())
		if err != nil {
			writeError(w, http.StatusConflict, err)
			return false
		}
	}
	// Both ownership and ticket freshness are checked after any queue wait.
	receipt := &commandReceipt{key: key, ID: id, Epoch: snapshot.Epoch, Generation: snapshot.Generation,
		Sequence: sequence, method: r.Method, route: r.URL.RequestURI()}
	err = s.controller.admitCommand(actor, snapshot.Generation, r.Header.Get(commandTicketHeader), func() error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		return s.commands.begin(commandScope{actor: actor, generation: snapshot.Generation}, receipt)
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return false
	}
	invocation.receipt = receipt
	invocation.scope = commandScope{actor: actor, generation: snapshot.Generation}
	if deferredMotionCommand(r.URL.Path) {
		// Inference may take minutes. Serialize admission and the later motion
		// application, while leaving the live-control lane free during inference.
		invocation.release()
		invocation.release = nil
	}
	w.Header().Set(commandIDHeader, id)
	w.Header().Set(commandSequenceHeader, strconv.FormatUint(sequence, 10))
	return true
}

func (s *Server) replayControlCommand(w http.ResponseWriter, r *http.Request, invocation *commandInvocation, receipt *commandReceipt) bool {
	if receipt.State != "complete" || !receipt.Replayable {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "command already received; inspect its receipt and current state instead of resending", "receipt": receipt})
		return false
	}
	if receipt.method != r.Method || receipt.route != r.URL.RequestURI() || !invocation.body.matches(receipt) {
		writeError(w, http.StatusConflict, errors.New("command ID was already used for a different request"))
		return false
	}
	w.Header().Set(commandIDHeader, receipt.ID)
	w.Header().Set("X-MagicHandy-Command-Replayed", "true")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(receipt.HTTPStatus)
	_, _ = w.Write(receipt.Response)
	return false
}

func (s *Server) handleCommandReceipt(w http.ResponseWriter, r *http.Request) {
	actor := controllerActor(r, clientIDFromRequest(r))
	if actor.sessionKey == "" {
		s.writeAuthenticationRequired(w)
		return
	}
	receipt := s.commands.lookup(commandReceiptKey{session: actor.sessionKey, client: actor.clientID, id: r.PathValue("id")})
	if receipt == nil {
		writeError(w, http.StatusNotFound, errors.New("no retained receipt for this session and tab; reconcile current state before retrying"))
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
