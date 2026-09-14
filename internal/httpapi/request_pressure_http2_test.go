package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestHTTP2TwentySpectatorsKeepStopAndRevocationResponsive(t *testing.T) {
	s, store, _, ownerCookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, ownerCookie)
	observer, err := store.Create(t.Context(), "pressure-spectator", "synthetic pressure passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternHardAndRegular, SpeedPercent: 20}, settings.Motion); err != nil {
		t.Fatal(err)
	}
	host := httptest.NewUnstartedServer(s.Handler())
	host.EnableHTTP2 = true
	host.StartTLS()
	defer host.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cookies := make([]*http.Cookie, 0, 20)
	streams := make([]*http.Response, 0, 20)
	var readers, requests sync.WaitGroup
	defer func() {
		cancel()
		for _, response := range streams {
			_ = response.Body.Close()
		}
		readers.Wait()
		requests.Wait()
	}()
	firstEnded := make(chan struct{})
	for index := range 20 {
		token, _, err := store.NewSession(ctx, observer.ID)
		if err != nil {
			t.Fatal(err)
		}
		cookie := testSessionCookie(token)
		cookies = append(cookies, cookie)
		response, reader := openPressureStream(ctx, t, host, cookie, index)
		streams = append(streams, response)
		readers.Add(1)
		go func(index int) {
			defer readers.Done()
			_, _ = io.Copy(io.Discard, reader)
			if index == 0 {
				close(firstEnded)
			}
		}(index)
	}
	var stateBytes atomic.Int64
	for index, cookie := range cookies {
		requests.Add(1)
		go func(index int, cookie *http.Cookie) {
			defer requests.Done()
			stateBytes.Add(readPressureState(ctx, t, host, cookie, index))
		}(index, cookie)
	}
	latencies := measurePressureStops(ctx, t, host, engine)
	requests.Wait()
	revoked := time.Now()
	if err := store.RevokeSession(ctx, cookies[0].Value); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstEnded:
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("revoked observer retained its HTTP/2 stream")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("loopback HTTP/2: spectators=20 state_requests=20 state_bytes=%d Stop_p50=%s Stop_p95=%s Stop_max=%s revocation=%s ordinary_peak=%d", stateBytes.Load(), latencies[9], latencies[18], latencies[19], time.Since(revoked), s.requestAdmission.snapshot()[ordinaryLane].Peak)
}

func measurePressureStops(ctx context.Context, t *testing.T, host *httptest.Server, engine *motion.Engine) []time.Duration {
	t.Helper()
	latencies := make([]time.Duration, 0, 20)
	for range 20 {
		started := time.Now()
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, host.URL+"/api/motion/stop", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := host.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		latencies = append(latencies, time.Since(started))
		if readErr != nil || response.StatusCode != http.StatusOK || engine.Snapshot().Running {
			t.Fatal("Stop failed with active spectators")
		}
	}
	return latencies
}

func readPressureState(ctx context.Context, t *testing.T, host *httptest.Server, cookie *http.Cookie, index int) int64 {
	t.Helper()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+"/api/state", nil)
	if err != nil {
		t.Error(err)
		return 0
	}
	r.AddCookie(cookie)
	r.Header.Set(controllerHeaderName, fmt.Sprintf("spectator-%d", index))
	response, err := host.Client().Do(r)
	if err != nil {
		t.Error(err)
		return 0
	}
	defer func() { _ = response.Body.Close() }()
	n, err := io.Copy(io.Discard, response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Errorf("spectator state: %d %v", response.StatusCode, err)
	}
	return n
}

func openPressureStream(ctx context.Context, t *testing.T, host *httptest.Server, cookie *http.Cookie, index int) (*http.Response, *bufio.Reader) {
	t.Helper()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+"/api/motion/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.AddCookie(cookie)
	r.Header.Set(controllerHeaderName, fmt.Sprintf("spectator-%d", index))
	response, err := host.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.ProtoMajor != 2 {
		_ = response.Body.Close()
		t.Fatal("spectator did not open an HTTP/2 stream")
	}
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = response.Body.Close()
			t.Fatal(err)
		}
		if line == "\n" {
			break
		}
	}
	return response, reader
}
