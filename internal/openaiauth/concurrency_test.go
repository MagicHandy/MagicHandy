package openaiauth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"
)

// Hold a fully received issuer response to exercise late completion even when
// the caller cancels; network cancellation alone is not the stale-result guard.
type gatedIssuer struct {
	path              string
	entered, release  chan struct{}
	once              sync.Once
	honorCancellation bool
}

func (g *gatedIssuer) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil || request.URL.Path != g.path {
		return response, err
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	g.once.Do(func() { close(g.entered) })
	var canceled <-chan struct{}
	if g.honorCancellation {
		canceled = request.Context().Done()
	}
	select {
	case <-g.release:
		return response, nil
	case <-canceled:
		return nil, request.Context().Err()
	}
}

func gateIssuer(t *testing.T, manager *Manager, path string) *gatedIssuer {
	t.Helper()
	gate := &gatedIssuer{path: path, entered: make(chan struct{}), release: make(chan struct{})}
	manager.client.Transport = gate
	t.Cleanup(func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
	})
	return gate
}

func awaitResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("operation stalled behind issuer network I/O")
	}
	var zero T
	return zero
}

func seededRefreshManager(t *testing.T, fixture *oauthFixture, directory string) *Manager {
	t.Helper()
	m := fixtureManager(t, fixture, directory)
	m.mu.Lock()
	err := m.transaction(t.Context(), func(data *credentialFile) error {
		data.Active = "first"
		data.Profiles = []registration{
			{ID: "first", ClientID: "first-client", AccessToken: "old-access", RefreshToken: "old-refresh", Scopes: []string{"chatgpt.tokens.use.direct"}, ExpiresAt: time.Now().Add(-time.Minute)},
			{ID: "second", ClientID: "second-client", AccessToken: "second-access", RefreshToken: "second-refresh", Scopes: []string{"chatgpt.tokens.use.direct"}, ExpiresAt: time.Now().Add(time.Hour)},
		}
		return nil
	})
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

type tokenResult struct {
	token string
	err   error
}

func beginRefresh(t *testing.T, m *Manager) <-chan tokenResult {
	t.Helper()
	source, err := m.ActiveTokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan tokenResult, 1)
	go func() { token, err := source(t.Context()); result <- tokenResult{token, err} }()
	return result
}

func promptStatus(t *testing.T, managers ...*Manager) {
	t.Helper()
	for _, m := range managers {
		result := make(chan Status, 1)
		started := time.Now()
		go func() { result <- m.Status(t.Context()) }()
		if status := awaitResult(t, result); status.State == "unavailable" {
			t.Fatal("status unavailable during network request")
		}
		t.Logf("status during blocked issuer: %s", time.Since(started))
	}
}

func TestBlockedRefreshKeepsStatusAndWelcomeResponsive(t *testing.T) {
	f := newOAuthFixture(t)
	dir := t.TempDir()
	m := seededRefreshManager(t, f, dir)
	other := fixtureManager(t, f, dir)
	gate := gateIssuer(t, m, "/token")
	result := beginRefresh(t, m)
	awaitResult(t, gate.entered)
	promptStatus(t, m, other)
	ack := make(chan error, 1)
	go func() { ack <- other.AcknowledgeWelcome(t.Context()) }()
	if err := awaitResult(t, ack); err != nil {
		t.Fatal(err)
	}
	close(gate.release)
	if got := awaitResult(t, result); got.err != nil || got.token != "renewed-access" {
		t.Fatal("welcome acknowledgment discarded rotated credentials", got.err)
	}
	if status := m.Status(t.Context()); status.Profiles[0].WelcomePending {
		t.Fatal("refresh lost the welcome acknowledgment")
	}
}

func TestLateRefreshCannotUndoSwitchOrSignOut(t *testing.T) {
	for _, operation := range []string{"switch", "other-process-switch-back", "sign-out"} {
		t.Run(operation, func(t *testing.T) {
			f := newOAuthFixture(t)
			dir := t.TempDir()
			m := seededRefreshManager(t, f, dir)
			other := fixtureManager(t, f, dir)
			gate := gateIssuer(t, m, "/token")
			result := beginRefresh(t, m)
			awaitResult(t, gate.entered)
			changed := make(chan error, 1)
			go func() {
				switch operation {
				case "switch":
					changed <- m.Select(t.Context(), "second")
				case "other-process-switch-back":
					if err := other.Select(t.Context(), "second"); err != nil {
						changed <- err
						return
					}
					changed <- other.Select(t.Context(), "first")
				case "sign-out":
					_, err := m.Disconnect(t.Context(), "first")
					changed <- err
				}
			}()
			if err := awaitResult(t, changed); err != nil {
				t.Fatal(err)
			}
			close(gate.release)
			if got := awaitResult(t, result); got.err == nil || got.token != "" {
				t.Fatal("stale token escaped after account change")
			}
			m.mu.Lock()
			err := m.transaction(t.Context(), func(data *credentialFile) error {
				if data.Profiles[0].AccessToken == "renewed-access" {
					t.Error("stale refresh was persisted")
				}
				if operation == "sign-out" && (data.Active != "" || data.Profiles[0].RefreshToken != "") {
					t.Error("logout was undone")
				}
				return nil
			})
			m.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRefreshWaitersShareRotationAndCanCancel(t *testing.T) {
	f := newOAuthFixture(t)
	m := seededRefreshManager(t, f, t.TempDir())
	gate := gateIssuer(t, m, "/token")
	first := beginRefresh(t, m)
	awaitResult(t, gate.entered)
	var waiters []<-chan tokenResult
	for range 8 {
		waiters = append(waiters, beginRefresh(t, m))
	}
	source, err := m.ActiveTokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	canceled := make(chan error, 1)
	go func() { _, err := source(ctx); canceled <- err }()
	cancel()
	if err := awaitResult(t, canceled); err == nil {
		t.Fatal("canceled waiter succeeded")
	}
	close(gate.release)
	for _, result := range append(waiters, first) {
		if got := awaitResult(t, result); got.err != nil || got.token != "renewed-access" {
			t.Fatal("shared refresh failed", got.err)
		}
	}
	if f.refreshes.Load() != 1 {
		t.Fatal("refresh was not shared")
	}
}

func TestBlockedRevocationClearsAuthorityBeforeNetworkCompletes(t *testing.T) {
	f := newOAuthFixture(t)
	f.revocationFails = true
	m := seededRefreshManager(t, f, t.TempDir())
	gate := gateIssuer(t, m, "/revoke")
	done := make(chan error, 1)
	go func() { _, err := m.Disconnect(t.Context(), "first"); done <- err }()
	awaitResult(t, gate.entered)
	promptStatus(t, m)
	if status := m.Status(t.Context()); status.Active != "" || status.Profiles[0].Connected {
		t.Fatal("logout retained authority during revocation")
	}
	if err := m.Select(t.Context(), "second"); err != nil {
		t.Fatal(err)
	}
	close(gate.release)
	if err := awaitResult(t, done); err != nil {
		t.Fatal(err)
	}
	if status := m.Status(t.Context()); status.Active != "second" || status.Message != "" {
		t.Fatal("late revocation overwrote selected account state")
	}
}

func TestBlockedDiscoveryCanBeCanceledWithoutStallingStatus(t *testing.T) {
	f := newOAuthFixture(t)
	m := fixtureManager(t, f, t.TempDir())
	gate := gateIssuer(t, m, "/.well-known/openid-configuration")
	done := make(chan error, 1)
	go func() { _, err := m.Start(t.Context(), ""); done <- err }()
	awaitResult(t, gate.entered)
	promptStatus(t, m)
	m.Cancel()
	close(gate.release)
	if err := awaitResult(t, done); err == nil {
		t.Fatal("canceled discovery started a callback listener")
	}
	if status := m.Status(t.Context()); status.Pending {
		t.Fatal("stale sign-in is pending")
	}
}

func TestBlockedCallbackCannotCompleteAfterCancellation(t *testing.T) {
	f := newOAuthFixture(t)
	m := fixtureManager(t, f, t.TempDir())
	callback, query := beginFixtureSignIn(t, m, f, "")
	gate := gateIssuer(t, m, "/token")
	callback.RawQuery = url.Values{"state": {query.Get("state")}, "code": {"test-code"}, "client_id": {"blocked-callback"}}.Encode()
	done := make(chan struct{})
	go func() {
		response, err := http.Get(callback.String()) // #nosec G107 -- fixture-owned loopback callback.
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		close(done)
	}()
	awaitResult(t, gate.entered)
	promptStatus(t, m)
	m.Cancel()
	close(gate.release)
	awaitResult(t, done)
	if status := m.Status(t.Context()); status.Active != "" || status.Pending {
		t.Fatal("canceled callback activated credentials")
	}
}

func TestCloseCancelsNetworkAndWaitsForRefresh(t *testing.T) {
	f := newOAuthFixture(t)
	m := seededRefreshManager(t, f, t.TempDir())
	gate := gateIssuer(t, m, "/token")
	gate.honorCancellation = true
	result := beginRefresh(t, m)
	awaitResult(t, gate.entered)
	done := make(chan struct{})
	go func() { m.Close(); close(done) }()
	awaitResult(t, done)
	if got := awaitResult(t, result); got.err == nil || got.token != "" {
		t.Fatal("closed manager issued credentials")
	}
}
