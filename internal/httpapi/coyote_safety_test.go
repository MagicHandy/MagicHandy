package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/estim"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type failingCoyoteZero struct {
	*estim.SimulatedConnector
	fail    atomic.Bool
	entered chan struct{}
	resume  chan struct{}
}

func (c *failingCoyoteZero) Connect(ctx context.Context, request estim.ConnectRequest) (estim.Link, error) {
	link, err := c.SimulatedConnector.Connect(ctx, request)
	return &failingCoyoteZeroLink{Link: link, connector: c}, err
}

type failingCoyoteZeroLink struct {
	estim.Link
	connector *failingCoyoteZero
}

func (l *failingCoyoteZeroLink) Write(ctx context.Context, payload []byte, ack bool) error {
	packet, err := estim.DecodePacket(payload)
	if err == nil && packet.A.Mode == estim.StrengthSet && packet.A.Value == 0 && l.connector.fail.Load() {
		select {
		case l.connector.entered <- struct{}{}:
		default:
		}
		select {
		case <-l.connector.resume:
		case <-ctx.Done():
			return ctx.Err()
		}
		return errors.New("test Coyote zero write failed")
	}
	return l.Link.Write(ctx, payload, ack)
}

func TestCoyoteFailedZeroReachesEveryOverlappingStopReply(t *testing.T) {
	connector := &failingCoyoteZero{SimulatedConnector: estim.NewSimulatedConnector(), entered: make(chan struct{}, 1), resume: make(chan struct{})}
	handy := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{CoyoteConnector: connector, Transport: handy, MotionTransport: handy})
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Device = settings.Device.WithPlaybackOutput(config.DeviceOutputCoyote)
		settings.Estim.SafetyAcknowledged = true
		return settings
	})
	connectCoyote(t, s)
	connector.fail.Store(true)
	stop := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		s.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
		return recorder
	}
	leader, follower := make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)
	go func() { leader <- stop() }()
	select {
	case <-connector.entered:
	case <-time.After(time.Second):
		t.Fatal("Stop did not issue zero")
	}
	go func() { follower <- stop() }()
	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		s.stopAdmission.mu.Lock()
		shared := s.stopAdmission.shared
		s.stopAdmission.mu.Unlock()
		if shared > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second Stop was not coalesced")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-follower:
		t.Fatal("Stop follower completed before the zero result")
	default:
	}
	close(connector.resume)
	for _, response := range []*httptest.ResponseRecorder{<-leader, <-follower} {
		if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "error") {
			t.Fatalf("failed zero was reported as confirmed: %d %s", response.Code, response.Body.String())
		}
	}
	if status := s.coyote.owner.Status(); status.Connected || status.StrengthKnown {
		t.Fatalf("failed zero retained a usable or confirmed connection: %+v", status)
	}
}

func TestCoyoteLoweringCannotPlayAndTestingRequiresTheControllingHost(t *testing.T) {
	s, connector := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	connectCoyote(t, s)
	waitCoyoteStatus(t, s, func(status transport.CoyoteStatus) bool { return status.StrengthKnown && !status.StrengthPending })
	if code, _ := coyoteRequest(t, s, http.MethodPost, "/api/transport/coyote/strength", `{"channel":"a","delta":5}`, true); code != http.StatusOK {
		t.Fatalf("raise=%d", code)
	}
	waitCoyoteStatus(t, s, func(status transport.CoyoteStatus) bool { return status.StrengthA == 5 })
	request := func(path, body, client string, sequence uint64) int {
		r := withControllerID(httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)), client)
		r.Header.Set(stopSequenceHeader, strconv.FormatUint(sequence, 10))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if code := request("/api/transport/coyote/test", `{"channel":"a"}`, "another-browser", 0); code == http.StatusOK {
		t.Fatal("a non-controlling client started a test")
	}
	if code := request("/api/transport/coyote/strength", `{"channel":"a","delta":-1}`, "another-browser", 0); code != http.StatusOK {
		t.Fatalf("safety decrease was blocked: %d", code)
	}
	waitCoyoteStatus(t, s, func(status transport.CoyoteStatus) bool { return status.StrengthA == 4 })
	if device := connector.LastLink().Snapshot(); device.OutputA != 0 || device.OutputB != 0 {
		t.Fatalf("a safety decrease started output: %+v", device)
	}
	if code := request("/api/transport/coyote/test", `{"channel":"a"}`, "test-controller", 0); code != http.StatusOK {
		t.Fatalf("explicit host test rejected: %d", code)
	}
	waitCoyoteStatus(t, s, func(status transport.CoyoteStatus) bool { return status.CommandedA > 0 })
	_, _ = s.emergencyStop(context.Background(), "test")
	if code := request("/api/transport/coyote/test", `{"channel":"a"}`, "test-controller", 0); code != http.StatusConflict {
		t.Fatalf("a stale test after Stop was accepted: %d", code)
	}
	if code := request("/api/transport/coyote/strength", `{"channel":"a","delta":1}`, "test-controller", 0); code != http.StatusConflict {
		t.Fatalf("a stale increase after Stop was accepted: %d", code)
	}
	if device := connector.LastLink().Snapshot(); device.StrengthA != 0 || device.OutputA != 0 {
		t.Fatalf("output survived Stop: %+v", device)
	}
}
