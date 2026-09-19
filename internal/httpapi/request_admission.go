package httpapi

import (
	"net/http"
	"strings"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

type requestLane int

const (
	ordinaryLane requestLane = iota
	loginLane
	shellLane
	livenessLane
	controlLane
	requestLaneCount
)

type requestLaneLimit struct {
	name         string
	global, peer int
}

var requestLaneLimits = [requestLaneCount]requestLaneLimit{
	{name: "ordinary", global: 128, peer: 64},
	{name: "login", global: 8, peer: 2},
	{name: "shell", global: 16, peer: 8},
	{name: "liveness", global: 16, peer: 8},
	{name: "control", global: 16, peer: 8},
}

type requestLaneState struct {
	active, peak int
	rejected     uint64
	peers        map[string]int
}

// requestAdmissionRuntime bounds work before any account/datastore lookup.
// It has no waiting queue. Peer entries live only while a request is admitted.
type requestAdmissionRuntime struct {
	mu    sync.Mutex
	lanes [requestLaneCount]requestLaneState
}

type requestLaneStatus struct {
	Lane         string `json:"lane"`
	Active       int    `json:"active"`
	Peak         int    `json:"peak_since_startup"`
	Limit        int    `json:"limit"`
	PerPeerLimit int    `json:"per_peer_limit"`
	Rejected     uint64 `json:"rejected_since_startup"`
}

func (a *requestAdmissionRuntime) enter(lane requestLane, peer string) (func(), bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, limit := &a.lanes[lane], requestLaneLimits[lane]
	if state.active >= limit.global || state.peers[peer] >= limit.peer {
		state.rejected++
		return nil, false
	}
	if state.peers == nil {
		state.peers = make(map[string]int)
	}
	state.active++
	state.peak = max(state.peak, state.active)
	state.peers[peer]++
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		state.active--
		state.peers[peer]--
		if state.peers[peer] == 0 {
			delete(state.peers, peer)
		}
	}, true
}

func (a *requestAdmissionRuntime) snapshot() []requestLaneStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]requestLaneStatus, 0, requestLaneCount)
	for lane, state := range a.lanes {
		limit := requestLaneLimits[lane]
		result = append(result, requestLaneStatus{Lane: limit.name, Active: state.active, Peak: state.peak, Limit: limit.global, PerPeerLimit: limit.peer, Rejected: state.rejected})
	}
	return result
}

func (s *Server) admitHTTPRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if readRequest(r) {
			finishUnreadBody(w, r)
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/motion/stop" {
			next.ServeHTTP(w, r)
			return
		}
		lane := s.requestLane(r)
		leave, admitted := s.requestAdmission.enter(lane, netaccess.ClientIP(r))
		if !admitted {
			finishUnreadBody(w, r)
			w.Header().Set("Retry-After", "1")
			writeBoundedJSON(w, http.StatusTooManyRequests, map[string]string{"error": "server request capacity is busy; retry shortly"})
			return
		}
		defer leave()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLane(r *http.Request) requestLane {
	if readRequest(r) && !strings.HasPrefix(r.URL.Path, "/api/") {
		return shellLane
	}
	if r.Method == http.MethodPost && (r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/bootstrap" || r.URL.Path == "/api/auth/recover") {
		return loginLane
	}
	if r.URL.Path == "/api/controller/heartbeat" || immediateControlTraffic(r) {
		owner := s.controller.Owner()
		if cleanControllerClientID(r.Header.Get(controllerHeaderName)) == owner.clientID && s.matchesRequestSession(r, owner.sessionKey) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/controller/heartbeat" {
				return livenessLane
			}
			if immediateControlTraffic(r) {
				return controlLane
			}
		}
	}
	if bluetoothGatewayDataRoute(r) && s.matchesRequestSession(r, s.bluetoothGatewaySessionKey()) {
		return livenessLane
	}
	return ordinaryLane
}

func immediateControlTraffic(r *http.Request) bool {
	if readRequest(r) {
		return r.URL.Path == "/api/state" || r.URL.Path == "/api/controller"
	}
	return controlRoute(r) && motionCommandRoute(r.URL.Path) && !deferredMotionCommand(r.URL.Path)
}

func (s *Server) matchesRequestSession(r *http.Request, key string) bool {
	if key == "" {
		return false
	}
	cookie, err := r.Cookie(s.sessionCookieName())
	return err == nil && accounts.MatchesSessionKey(strings.TrimSpace(cookie.Value), key)
}
