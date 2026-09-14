package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const gatewayTestClient = "device-browser"
const gatewayTestTab = "test-controller"

func newGatewaySessionFixture(t *testing.T) (*Server, *accounts.Store, accounts.Account, *http.Cookie, bluetoothGatewaySnapshot) {
	t.Helper()
	s, store, admin, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	return s, store, admin, cookie, connectGatewayFixture(t, s, cookie)
}

func connectGatewayFixture(t *testing.T, s *Server, cookie *http.Cookie) bluetoothGatewaySnapshot {
	t.Helper()
	response := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/transport/bluetooth/connect", gatewayTestTab,
		`{"client_id":"device-browser","connected":true,"supported":true}`)
	var result bluetoothStatusResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !result.Gateway.Required || !result.Gateway.Owned || result.Gateway.Generation == 0 {
		t.Fatalf("gateway setup failed: %d %s", response.Code, response.Body.String())
	}
	if result.Bluetooth.ClientID != gatewayTestClient {
		t.Fatal("public status lost the browser's ordinary ID")
	}
	return result.Gateway
}

func gatewayFixtureRequest(cookie *http.Cookie, tab, method, route, body string, gateway bluetoothGatewaySnapshot) *http.Request {
	r := httptest.NewRequest(method, route, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(controllerHeaderName, tab)
	r.Header.Set(gatewayEpochHeader, gateway.Epoch)
	r.Header.Set(gatewayGenerationHeader, strconv.FormatUint(gateway.Generation, 10))
	r.AddCookie(cookie)
	return r
}

func serveGatewayFixture(s *Server, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func queueGatewayFixture(t *testing.T, s *Server, path string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("fake gateway command did not stop")
		}
	})
	go func() {
		defer close(done)
		_ = s.bluetooth.bridge.SendCommand(ctx, transport.CommandKind("read"), path, nil)
	}()
	deadline := time.Now().Add(time.Second)
	for s.bluetooth.bridge.Snapshot().Pending+s.bluetooth.bridge.Snapshot().Inflight == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.bluetooth.bridge.Snapshot().Pending+s.bluetooth.bridge.Snapshot().Inflight == 0 {
		t.Fatal("fake gateway command was not queued")
	}
}

func TestBluetoothGatewayRejectsCopiedIdentity(t *testing.T) {
	for _, actor := range []string{"observer", "same-account-other-session", "same-session-other-tab"} {
		t.Run(actor, func(t *testing.T) {
			s, store, admin, cookie, gateway := newGatewaySessionFixture(t)
			otherCookie, otherTab := cookie, gatewayTestTab
			if actor == "same-session-other-tab" {
				otherTab = "other-tab"
			} else {
				account := admin
				if actor == "observer" {
					var err error
					account, err = store.Create(t.Context(), "observer", "synthetic observer test account", accounts.RoleOperator)
					if err != nil {
						t.Fatal(err)
					}
				}
				token, _, err := store.NewSession(t.Context(), account.ID)
				if err != nil {
					t.Fatal(err)
				}
				otherCookie = testSessionCookie(token)
			}
			queueGatewayFixture(t, s, "hsp/state")
			response := serveGatewayFixture(s, gatewayFixtureRequest(otherCookie, otherTab, http.MethodGet,
				"/api/transport/bluetooth/commands?client_id=device-browser&wait=0", "", gateway))
			if response.Code != http.StatusForbidden || s.bluetooth.bridge.Snapshot().Pending != 1 {
				t.Fatalf("copied ID drained the gateway: %d %s", response.Code, response.Body.String())
			}
			legitimate := serveGatewayFixture(s, gatewayFixtureRequest(cookie, gatewayTestTab, http.MethodGet,
				"/api/transport/bluetooth/commands?client_id=device-browser&wait=0", "", gateway))
			var commands bluetoothCommandsResponse
			if err := json.Unmarshal(legitimate.Body.Bytes(), &commands); err != nil {
				t.Fatal(err)
			}
			if len(commands.Commands) != 1 {
				t.Fatal("legitimate gateway lost its command")
			}
			s.bluetooth.gateway.mu.Lock()
			lastSeen := s.bluetooth.gateway.lease.lastSeen
			s.bluetooth.gateway.mu.Unlock()
			for _, request := range []struct{ route, body string }{
				{"/api/transport/bluetooth/ack", `{"client_id":"device-browser","id":"` + commands.Commands[0].ID + `","ok":true}`},
				{"/api/transport/bluetooth/status", `{"client_id":"device-browser","connected":false}`},
			} {
				response = serveGatewayFixture(s, gatewayFixtureRequest(otherCookie, otherTab, http.MethodPost, request.route, request.body, gateway))
				if response.Code != http.StatusForbidden {
					t.Fatalf("forged gateway write: %d %s", response.Code, response.Body.String())
				}
			}
			s.bluetooth.gateway.mu.Lock()
			unchanged := s.bluetooth.gateway.lease.lastSeen.Equal(lastSeen)
			s.bluetooth.gateway.mu.Unlock()
			if !unchanged || !s.bluetooth.bridge.Snapshot().Connected || s.bluetooth.bridge.Snapshot().Inflight != 1 {
				t.Fatal("forged gateway write changed liveness, connection or acknowledgement")
			}
		})
	}
}

func TestBluetoothGatewaySurvivesControllerHandoff(t *testing.T) {
	s, store, admin, cookie, gateway := newGatewaySessionFixture(t)
	otherToken, _, err := store.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request := gatewayFixtureRequest(cookie, gatewayTestTab, http.MethodGet,
		"/api/transport/bluetooth/commands?client_id=device-browser&wait=4", "", gateway).WithContext(ctx)
	done := make(chan *httptest.ResponseRecorder, 1)
	s.bluetooth.gateway.mu.Lock()
	s.bluetooth.gateway.lease.lastSeen = time.Now().Add(-time.Second)
	before := s.bluetooth.gateway.lease.lastSeen
	s.bluetooth.gateway.mu.Unlock()
	go func() { done <- serveGatewayFixture(s, request) }()
	deadline := time.Now().Add(time.Second)
	for {
		s.bluetooth.gateway.mu.Lock()
		started := s.bluetooth.gateway.lease.lastSeen.After(before)
		s.bluetooth.gateway.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway poll did not start")
		}
		time.Sleep(time.Millisecond)
	}
	response := authenticatedControlRequest(s, testSessionCookie(otherToken), http.MethodPost, "/api/controller/takeover", "remote-controller", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("remote takeover: %d %s", response.Code, response.Body.String())
	}
	select {
	case <-done:
		t.Fatal("handoff canceled the device browser's pending poll")
	default:
	}
	queueGatewayFixture(t, s, "hsp/stop")
	select {
	case response = <-done:
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "hsp/stop") {
			t.Fatal("gateway could not receive Stop after handoff")
		}
	case <-time.After(time.Second):
		t.Fatal("gateway did not receive the queued Stop")
	}
	response = serveGatewayFixture(s, gatewayFixtureRequest(cookie, gatewayTestTab, http.MethodPost,
		"/api/transport/bluetooth/status", `{"client_id":"device-browser","connected":true}`, gateway))
	if response.Code != http.StatusOK {
		t.Fatalf("read-only device browser lost its gateway: %d", response.Code)
	}
}

func TestBluetoothGatewayReconnectionRejectsPreviousConnection(t *testing.T) {
	s, _, _, cookie, previous := newGatewaySessionFixture(t)
	oldTransportID := s.bluetooth.bridge.Snapshot().ClientID
	response := serveGatewayFixture(s, gatewayFixtureRequest(cookie, gatewayTestTab, http.MethodPost,
		"/api/transport/bluetooth/disconnect", `{"client_id":"device-browser"}`, previous))
	if response.Code != http.StatusOK {
		t.Fatalf("gateway disconnect: %d %s", response.Code, response.Body.String())
	}
	response = authenticatedControlRequest(s, cookie, http.MethodPost, "/api/controller/takeover", gatewayTestTab, `{}`)
	if response.Code != http.StatusOK {
		t.Fatal("explicit control recovery failed")
	}
	current := connectGatewayFixture(t, s, cookie)
	if current.Generation <= previous.Generation || s.bluetooth.bridge.Snapshot().ClientID == oldTransportID {
		t.Fatal("gateway connection identity was reused")
	}
	queueGatewayFixture(t, s, "hsp/state")
	for _, request := range []struct{ method, route, body string }{
		{http.MethodGet, "/api/transport/bluetooth/commands?client_id=device-browser&wait=0", ""},
		{http.MethodPost, "/api/transport/bluetooth/status", `{"client_id":"device-browser","connected":false}`},
		{http.MethodPost, "/api/transport/bluetooth/disconnect", `{"client_id":"device-browser"}`},
		{http.MethodPost, "/api/transport/bluetooth/ack", `{"client_id":"device-browser","id":"bt-000001","ok":true}`},
	} {
		response = serveGatewayFixture(s, gatewayFixtureRequest(cookie, gatewayTestTab, request.method, request.route, request.body, previous))
		if response.Code != http.StatusConflict {
			t.Fatalf("old connection admitted: %d %s", response.Code, response.Body.String())
		}
	}
	commands, err := s.bluetooth.bridge.NextCommands(t.Context(), oldTransportID, 0)
	if err != nil || len(commands) != 0 || s.bluetooth.bridge.Snapshot().Pending != 1 {
		t.Fatal("retired transport poll drained replacement work")
	}
	wrongEpoch := current
	wrongEpoch.Epoch = "previous-server"
	response = serveGatewayFixture(s, gatewayFixtureRequest(cookie, gatewayTestTab, http.MethodGet,
		"/api/transport/bluetooth/commands?client_id=device-browser&wait=0", "", wrongEpoch))
	if response.Code != http.StatusConflict {
		t.Fatal("old process epoch was admitted")
	}
}

func TestBluetoothGatewayLossStopsSharedEngine(t *testing.T) {
	for _, cause := range []string{"expired", "revoked", "disconnected"} {
		t.Run(cause, func(t *testing.T) {
			s, store, _, cookie, gateway := newGatewaySessionFixture(t)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.Device.HSPDispatchOwner = config.DispatchOwnerBrowserBluetooth
				return settings
			})
			response := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/motion/start", gatewayTestTab, `{"speed_percent":20}`)
			if response.Code != http.StatusOK {
				t.Fatalf("fake engine start failed: %d %s", response.Code, response.Body.String())
			}
			engine := s.currentMotionEngine()
			switch cause {
			case "expired":
				s.bluetooth.gateway.mu.Lock()
				s.bluetooth.gateway.lease.lastSeen = time.Now().Add(-gatewayLeaseTTL)
				s.bluetooth.gateway.mu.Unlock()
				response = serveGatewayFixture(s, gatewayFixtureRequest(cookie, gatewayTestTab, http.MethodPost,
					"/api/transport/bluetooth/status", `{"client_id":"device-browser","connected":true}`, gateway))
				if response.Code != http.StatusConflict {
					t.Fatal("expired gateway renewed itself")
				}
			case "revoked":
				if err := store.RevokeSession(t.Context(), cookie.Value); err != nil {
					t.Fatal(err)
				}
			case "disconnected":
				response = serveGatewayFixture(s, gatewayFixtureRequest(cookie, gatewayTestTab, http.MethodPost,
					"/api/transport/bluetooth/status", `{"client_id":"device-browser","connected":false}`, gateway))
				if response.Code != http.StatusOK {
					t.Fatal("gateway could not report its disconnect")
				}
			}
			s.checkAccessLifetimes()
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) && (engine.Snapshot().Running || s.bluetoothGatewaySessionKey() != "") {
				time.Sleep(time.Millisecond)
			}
			if engine.Snapshot().Running || s.bluetooth.bridge.Snapshot().Connected || s.stopSequence.Load() == 0 || s.bluetoothGatewaySessionKey() != "" {
				t.Fatal("gateway loss did not stop and release the shared engine")
			}
		})
	}
}
