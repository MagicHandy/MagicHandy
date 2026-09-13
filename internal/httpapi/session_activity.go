package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

const (
	accessWatchdogInterval = time.Second
	shutdownWriteGrace     = 5 * time.Second
)

type controllerCancellationKey struct{}
type sessionCancellationKey struct{}

type sessionActivity struct {
	ctx      context.Context
	cancel   context.CancelFunc
	requests int
}

type sessionActivityRuntime struct {
	mu       sync.Mutex
	sessions map[string]*sessionActivity
	closing  bool
}

func newSessionActivityRuntime() sessionActivityRuntime {
	return sessionActivityRuntime{sessions: make(map[string]*sessionActivity)}
}

func (a *sessionActivityRuntime) enter(key string) (*sessionActivity, func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry := a.sessions[key]
	if entry == nil {
		// Bound simultaneous active sessions and per-session streams/requests.
		if len(a.sessions) >= 128 {
			return nil, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		entry = &sessionActivity{ctx: ctx, cancel: cancel}
		a.sessions[key] = entry
	}
	if entry.requests >= 32 || entry.ctx.Err() != nil {
		return nil, nil
	}
	entry.requests++
	return entry, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		entry.requests--
		if entry.requests == 0 {
			entry.cancel()
			if a.sessions[key] == entry {
				delete(a.sessions, key)
			}
		}
	}
}

func (a *sessionActivityRuntime) keys() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	keys := make([]string, 0, len(a.sessions))
	for key := range a.sessions {
		keys = append(keys, key)
	}
	return keys
}

func (a *sessionActivityRuntime) revoke(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if entry := a.sessions[key]; entry != nil {
		entry.cancel()
	}
}

func (s *Server) trackSessionActivity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, authenticated := authenticatedSession(r)
		// Stop never waits on session bookkeeping, database revalidation, or an
		// exhausted ordinary request budget. Its public authority is unchanged.
		if isPublicAuthenticationRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		lifetime := s.auth.unprotectedCtx
		if authenticated {
			entry, leave := s.access.enter(session.session.Key)
			if entry == nil {
				writeError(w, http.StatusTooManyRequests, errors.New("too many active requests; close an unused tab and retry"))
				return
			}
			defer leave()
			lifetime = entry.ctx
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		stopSession := context.AfterFunc(lifetime, cancel)
		defer stopSession()
		ctx = context.WithValue(ctx, sessionCancellationKey{}, stopSession)
		if lifetime.Err() != nil {
			cancel()
		}
		stopServer := context.AfterFunc(s.lifecycleCtx, cancel)
		defer stopServer()
		if authenticated && r.URL.Path != "/api/controller/takeover" {
			binding := &controllerRequestBinding{cancel: cancel}
			// The device browser must still deliver Stop when a remote controller
			// takes over. Its gateway traffic is bound to its login and gateway,
			// while a handler may explicitly bind an administrative control action.
			if !bluetoothGatewaySessionRoute(r) {
				binding.bind(&s.controller, controllerActor(r, clientIDFromRequest(r)))
			}
			defer binding.release()
			ctx = context.WithValue(ctx, controllerRequestBindingKey{}, binding)
			ctx = context.WithValue(ctx, controllerCancellationKey{}, binding.release)
		}
		// A canceled stream must also release a blocked socket write/body read.
		// Deadline support is optional only for in-memory test response writers.
		interrupted := make(chan struct{})
		interrupt := context.AfterFunc(ctx, func() {
			defer close(interrupted)
			now := time.Now()
			writeDeadline := now
			if s.quiescing.Load() {
				// Healthy shutdown must finish HTTP framing after the handler
				// returns. Keep that write bounded for stalled receivers, while
				// live-server session/ownership revocation still interrupts now.
				writeDeadline = now.Add(shutdownWriteGrace)
			}
			_ = http.NewResponseController(w).SetReadDeadline(now)
			_ = http.NewResponseController(w).SetWriteDeadline(writeDeadline)
			if r.Body != nil {
				_ = r.Body.Close()
			}
		})
		defer func() {
			if !interrupt() {
				<-interrupted
			}
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) startAccessWatchdog() {
	s.accessWG.Add(1)
	go func() {
		defer s.accessWG.Done()
		ticker := time.NewTicker(accessWatchdogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.lifecycleCtx.Done():
				for _, key := range s.access.keys() {
					s.access.revoke(key)
				}
				return
			case <-ticker.C:
				s.commands.prune()
				s.checkAccessLifetimes()
			}
		}
	}()
}

func (s *Server) checkAccessLifetimes() {
	s.checkBluetoothGatewayLifetime("")
	if s.controller.BeginLoss("", true) {
		s.scheduleLostControllerStop("controller_heartbeat_expired")
	}
	keys := s.access.keys()
	if owner := s.controller.SessionKey(); owner != "" {
		keys = append(keys, owner)
	}
	if gateway := s.bluetoothGatewaySessionKey(); gateway != "" {
		keys = append(keys, gateway)
	}
	// Bound the entire validation pass. Storage unavailability fails closed;
	// it must not indefinitely preserve authority or block Emergency Stop.
	ctx, cancel := context.WithTimeout(s.lifecycleCtx, 500*time.Millisecond)
	defer cancel()
	checked := make(map[string]bool, len(keys))
	for _, key := range keys {
		if checked[key] {
			continue
		}
		checked[key] = true
		if session, err := s.accounts.CheckSession(ctx, key); err == nil {
			owner := s.controller.Owner()
			grantID := ""
			if session.ControlGrant != nil {
				grantID = session.ControlGrant.ID
			}
			if owner.sessionKey == key && (!session.CanControl(time.Now()) || owner.grantID != grantID) && s.controller.BeginLoss(key, false) {
				s.scheduleLostControllerStop("controller_permission_ended")
			}
			continue
		}
		s.access.revoke(key)
		s.checkBluetoothGatewayLifetime(key)
		if s.controller.BeginLoss(key, false) {
			s.scheduleLostControllerStop("controller_session_expired")
		}
	}
}

func (s *Server) scheduleLostControllerStop(reason string) {
	// BeginLoss admits at most one such worker. Keep validating other active
	// sessions while a slow physical transport is stopping the lost controller.
	s.access.mu.Lock()
	if s.access.closing {
		s.access.mu.Unlock()
		return
	}
	s.accessWG.Add(1)
	s.access.mu.Unlock()
	go func() {
		defer s.accessWG.Done()
		s.stopLostController(reason)
	}()
}

func (s *Server) closeAccessWorkers() {
	// Requests may trigger immediate grant revalidation during shutdown. Fence
	// every possible Add before waiting, including after the watchdog exits.
	s.access.mu.Lock()
	s.access.closing = true
	s.access.mu.Unlock()
	s.accessWG.Wait()
}

func (s *Server) stopLostController(reason string) {
	defer s.controller.FinishLoss()
	generation, grantID := s.controller.lossAuditReference()
	s.recordAccessEvent(context.Background(), audit.Event{Kind: audit.ControlLost, Outcome: "success", Operation: auditStopOperation(reason), Generation: generation, GrantID: grantID})
	ctx, cancel := context.WithTimeout(s.lifecycleCtx, 5*time.Second)
	defer cancel()
	_, err := s.emergencyStop(ctx, reason)
	if err != nil && s.lifecycleCtx.Err() == nil {
		s.logger.Warn("controller access ended; physical Stop could not be confirmed", "reason", reason)
	}
}
