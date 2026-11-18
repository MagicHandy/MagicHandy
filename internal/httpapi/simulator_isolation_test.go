package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestMissingSimulatorEndpointsDoNotDialConfiguredOwners(t *testing.T) {
	fake := transport.NewFake()
	intercept := &http.Client{Transport: modelRoutingTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("simulation attempted an external connection")
		return nil, context.Canceled
	})}
	s := newTestServerWithRuntime(t, Runtime{MotionSimulated: true, MotionTransport: fake, Transport: fake, IntifaceHTTPClient: intercept, AutoblowClient: autoblow.Options{HTTPClient: intercept}})
	t.Cleanup(s.Close)
	if err := s.openIntifaceHub(t.Context(), "ws://127.0.0.1:41234"); err == nil {
		t.Fatal("missing Intiface simulator appeared ready")
	}
	addVacuGlide(t, s, false)
	saveVacuGlideToken(t, s)
	code, _ := autoblowRequest(t, s, http.MethodPost, "/api/transport/autoblow/connect", `{"device":"vacuglide"}`, true)
	if code != http.StatusBadGateway {
		t.Fatalf("missing Autoblow simulator returned %d", code)
	}
}
