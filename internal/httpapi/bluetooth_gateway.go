package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	gatewayEpochHeader      = "X-MagicHandy-Gateway-Epoch"
	gatewayGenerationHeader = "X-MagicHandy-Gateway-Generation"
	gatewayLeaseTTL         = 10 * time.Second
)

type bluetoothGatewayRuntime struct {
	mu       sync.Mutex
	sequence uint64
	lease    *bluetoothGatewayLease
	stopping bool
}

// Gateway authority belongs to the authenticated device browser, independently
// of the remote controller. The transport sees only a per-connection identity.
type bluetoothGatewayLease struct {
	sessionKey  string
	tabID       string
	clientID    string
	transportID string
	epoch       string
	generation  uint64
	lastSeen    time.Time
	ctx         context.Context
	cancel      context.CancelFunc
}

type bluetoothGatewaySnapshot struct {
	Required             bool   `json:"required"`
	Owned                bool   `json:"owned"`
	Epoch                string `json:"epoch,omitempty"`
	Generation           uint64 `json:"generation,omitempty"`
	LeaseExpiresInMillis int64  `json:"lease_expires_in_ms,omitempty"`
}

func bluetoothGatewayDataRoute(r *http.Request) bool {
	return (r.Method == http.MethodGet && r.URL.Path == "/api/transport/bluetooth/commands") ||
		(r.Method == http.MethodPost && (r.URL.Path == "/api/transport/bluetooth/status" || r.URL.Path == "/api/transport/bluetooth/ack"))
}

func bluetoothGatewaySessionRoute(r *http.Request) bool {
	return bluetoothGatewayDataRoute(r) || (r.Method == http.MethodPost && r.URL.Path == "/api/transport/bluetooth/disconnect")
}

func (l *bluetoothGatewayLease) matches(r *http.Request, clientID string) bool {
	session, ok := authenticatedSession(r)
	return ok && session.session.Key == l.sessionKey && cleanControllerClientID(r.Header.Get(controllerHeaderName)) == l.tabID &&
		(clientID == "" || clientID == l.clientID)
}

func (l *bluetoothGatewayLease) current(r *http.Request) bool {
	generation, err := strconv.ParseUint(r.Header.Get(gatewayGenerationHeader), 10, 64)
	return err == nil && generation == l.generation && r.Header.Get(gatewayEpochHeader) == l.epoch &&
		l.ctx.Err() == nil && r.Context().Err() == nil && time.Since(l.lastSeen) < gatewayLeaseTTL
}

func (s *Server) bluetoothGatewaySnapshot(r *http.Request) bluetoothGatewaySnapshot {
	result := bluetoothGatewaySnapshot{Required: s.auth.authenticationRequired()}
	g := &s.bluetooth.gateway
	g.mu.Lock()
	defer g.mu.Unlock()
	if l := g.lease; l != nil && l.matches(r, "") && l.ctx.Err() == nil && time.Since(l.lastSeen) < gatewayLeaseTTL {
		result.Owned = true
		result.Epoch, result.Generation = l.epoch, l.generation
		result.LeaseExpiresInMillis = max(0, gatewayLeaseTTL.Milliseconds()-time.Since(l.lastSeen).Milliseconds())
	}
	return result
}

// Internal transport identities must not become browser credentials or leak
// session keys. Public status retains the ordinary, non-authoritative tab ID.
func (s *Server) bluetoothSnapshot() transport.BrowserBluetoothBridgeSnapshot {
	g := &s.bluetooth.gateway
	g.mu.Lock()
	defer g.mu.Unlock()
	snapshot := s.bluetooth.bridge.Snapshot()
	if l := g.lease; l != nil && snapshot.ClientID == l.transportID {
		snapshot.ClientID = l.clientID
	} else if s.auth.authenticationRequired() {
		snapshot.ClientID = ""
	}
	return snapshot
}

func (s *Server) registerBluetoothGateway(w http.ResponseWriter, r *http.Request, status transport.BrowserBluetoothClientStatus) bool {
	if !s.auth.authenticationRequired() {
		s.bluetooth.bridge.ConnectClient(status)
		return true
	}
	session, ok := authenticatedSession(r)
	tabID := cleanControllerClientID(r.Header.Get(controllerHeaderName))
	clientID := cleanControllerClientID(status.ClientID)
	if !ok || tabID == "" || clientID == "" || r.Context().Err() != nil {
		writeError(w, http.StatusForbidden, errors.New("a current authenticated browser is required for the device gateway"))
		return false
	}
	g := &s.bluetooth.gateway
	g.mu.Lock()
	if g.lease != nil || g.stopping || s.bluetooth.bridge.Snapshot().Connected {
		g.mu.Unlock()
		writeError(w, http.StatusConflict, errors.New("disconnect the current Bluetooth gateway before connecting another browser"))
		return false
	}
	g.sequence++
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
	l := &bluetoothGatewayLease{sessionKey: session.session.Key, tabID: tabID, clientID: clientID,
		epoch: s.controller.epoch, generation: g.sequence, lastSeen: time.Now(), ctx: ctx, cancel: cancel}
	l.transportID = l.epoch + "/" + strconv.FormatUint(l.generation, 10)
	g.lease = l
	status.ClientID = l.transportID
	s.bluetooth.bridge.ConnectClient(status)
	g.mu.Unlock()
	return true
}

// Hold this lock only for short bridge operations, never for a poll or write.
func (s *Server) lockBluetoothGateway(w http.ResponseWriter, r *http.Request, clientID string) (*bluetoothGatewayLease, func(), bool) {
	g := &s.bluetooth.gateway
	g.mu.Lock()
	l := g.lease
	if l == nil || clientID == "" || !l.matches(r, clientID) {
		g.mu.Unlock()
		writeError(w, http.StatusForbidden, errors.New("this session and tab do not own the Bluetooth gateway"))
		return nil, nil, false
	}
	if !l.current(r) {
		g.mu.Unlock()
		writeError(w, http.StatusConflict, errors.New("bluetooth gateway access ended; reconnect explicitly from the device browser"))
		return nil, nil, false
	}
	l.lastSeen = time.Now()
	return l, g.mu.Unlock, true
}

func (s *Server) bluetoothGatewaySessionKey() string {
	g := &s.bluetooth.gateway
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lease != nil {
		return g.lease.sessionKey
	}
	return ""
}

func (s *Server) beginBluetoothGatewayDisconnect(w http.ResponseWriter, r *http.Request, request bluetoothDisconnectRequest) (func(), bool) {
	if !s.auth.authenticationRequired() {
		return sync.OnceFunc(func() { s.bluetooth.bridge.DisconnectClient(request.ClientID, request.Message) }), true
	}
	g := &s.bluetooth.gateway
	g.mu.Lock()
	l := g.lease
	if l == nil {
		g.mu.Unlock()
		writeError(w, http.StatusConflict, errors.New("there is no current Bluetooth gateway to disconnect"))
		return nil, false
	}
	owned := l.matches(r, gatewayClientID(request.ClientID))
	if owned && !l.current(r) {
		g.mu.Unlock()
		writeError(w, http.StatusConflict, errors.New("bluetooth gateway access ended"))
		return nil, false
	}
	g.mu.Unlock()
	if !owned && (!s.capabilities(r).ConfigureHost || !s.requireController(w, r)) {
		if !s.capabilities(r).ConfigureHost {
			writeError(w, http.StatusForbidden, errors.New("only the gateway browser or controlling administrator can disconnect it"))
		}
		return nil, false
	}
	g.mu.Lock()
	if g.lease != l || g.stopping || r.Context().Err() != nil {
		g.mu.Unlock()
		writeError(w, http.StatusConflict, errors.New("bluetooth gateway changed while disconnecting"))
		return nil, false
	}
	g.stopping = true
	g.mu.Unlock()
	return sync.OnceFunc(func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.lease == l {
			s.bluetooth.bridge.DisconnectClient(l.transportID, request.Message)
			l.cancel()
			g.lease = nil
			g.stopping = false
		}
	}), true
}

func (s *Server) checkBluetoothGatewayLifetime(revokedSession string) {
	g := &s.bluetooth.gateway
	g.mu.Lock()
	l := g.lease
	if l == nil && s.auth.authenticationRequired() && s.bluetooth.bridge.Snapshot().Connected {
		// Enabling accounts cannot preserve an unbound, formerly local gateway.
		ctx, cancel := context.WithCancel(s.lifecycleCtx)
		l = &bluetoothGatewayLease{transportID: s.bluetooth.bridge.Snapshot().ClientID, ctx: ctx, cancel: cancel}
		g.lease = l
	}
	lost := l != nil && !g.stopping && (l.sessionKey == "" || l.ctx.Err() != nil ||
		time.Since(l.lastSeen) >= gatewayLeaseTTL || (revokedSession != "" && revokedSession == l.sessionKey))
	if !lost {
		g.mu.Unlock()
		return
	}
	g.stopping = true
	l.cancel()
	s.bluetooth.bridge.DisconnectClient(l.transportID, "Bluetooth gateway access ended; reconnect from the device browser.")
	g.mu.Unlock()
	s.finishLostBluetoothGateway(l)
}

func (s *Server) finishLostBluetoothGateway(l *bluetoothGatewayLease) {
	finish := func() {
		g := &s.bluetooth.gateway
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.lease == l {
			g.lease = nil
			g.stopping = false
		}
	}
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerBrowserBluetooth {
		finish()
		return
	}
	// Fence shared work synchronously. Only the existing engine Stop waits in a
	// tracked worker; a replacement gateway remains blocked until it finishes.
	s.access.mu.Lock()
	if s.access.closing {
		s.access.mu.Unlock()
		finish()
		return
	}
	s.accessWG.Add(1)
	s.access.mu.Unlock()
	finishStop, _ := s.beginGlobalStop("bluetooth_gateway_lost")
	go func() {
		defer s.accessWG.Done()
		defer finish()
		defer finishStop()
		ctx, cancel := context.WithTimeout(s.lifecycleCtx, 5*time.Second)
		defer cancel()
		if err := s.stopAndClearMotionEngine(ctx, "bluetooth_gateway_lost"); err != nil && s.lifecycleCtx.Err() == nil {
			s.logger.Warn("Bluetooth gateway lost; physical Stop could not be confirmed")
		}
	}()
}

func gatewayClientID(value string) string { return cleanControllerClientID(strings.TrimSpace(value)) }
