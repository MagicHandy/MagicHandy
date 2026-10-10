package openaiauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type oauthFixture struct {
	server          *httptest.Server
	key             *rsa.PrivateKey
	mu              sync.Mutex
	nonce           string
	claims          func(map[string]any)
	scope           string
	refreshScope    string
	refreshes       atomic.Int32
	revocationFails bool
}

func newOAuthFixture(t *testing.T) *oauthFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &oauthFixture{key: key, scope: "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *oauthFixture) serve(w http.ResponseWriter, r *http.Request) {
	base := f.server.URL
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		_ = json.NewEncoder(w).Encode(discovery{Issuer: base, AuthorizationEndpoint: base + "/authorize", TokenEndpoint: base + "/token", JWKSURI: base + "/jwks", RevocationEndpoint: base + "/revoke"})
	case "/jwks":
		e := big.NewInt(int64(f.key.PublicKey.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test-key", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(e)}}})
	case "/token":
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "refresh_token" {
			f.refreshes.Add(1)
			_ = json.NewEncoder(w).Encode(tokenReply{AccessToken: "renewed-access", RefreshToken: "rotated-refresh", ExpiresIn: 3600, TokenType: "Bearer", Scope: f.refreshScope}) // #nosec G117 -- synthetic OAuth fixture tokens sent only by its loopback test server.
			return
		}
		f.mu.Lock()
		nonce, mutate, scope := f.nonce, f.claims, f.scope
		f.mu.Unlock()
		claims := map[string]any{"iss": base, "sub": "verified-subject", "aud": r.Form.Get("client_id"), "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce, "email": "same@example.test"}
		if mutate != nil {
			mutate(claims)
		}
		_ = json.NewEncoder(w).Encode(tokenReply{AccessToken: "private-access", RefreshToken: "private-refresh", IDToken: f.signed(claims), ExpiresIn: 3600, TokenType: "Bearer", Scope: scope}) // #nosec G117 -- synthetic OAuth fixture tokens, no user credentials.
	case "/revoke":
		if f.revocationFails {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *oauthFixture) signed(claims map[string]any) string {
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key"})
	payload, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, hash[:])
	if err != nil {
		panic(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func fixtureManager(t *testing.T, fixture *oauthFixture, directory string) *Manager {
	t.Helper()
	manager, err := Open(Options{DataDir: directory, Issuer: fixture.server.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return manager
}

func beginFixtureSignIn(t *testing.T, m *Manager, f *oauthFixture, profile string) (*url.URL, url.Values) {
	t.Helper()
	authorization, err := m.Start(t.Context(), profile)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorization)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || !regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(query.Get("ext_agent_host_id")) || query.Get("nonce") == "" || query.Get("resource") != resourceURL {
		t.Fatal("incomplete OAuth request")
	}
	f.mu.Lock()
	f.nonce = query.Get("nonce")
	f.mu.Unlock()
	callback, err := url.Parse(query.Get("redirect_uri"))
	if err != nil || callback.Hostname() != "127.0.0.1" || callback.Path != "/auth/callback" {
		t.Fatal("invalid loopback callback")
	}
	return callback, query
}

func finishFixtureSignIn(t *testing.T, m *Manager, callback *url.URL, query url.Values, clientID string) Status {
	t.Helper()
	callback.RawQuery = url.Values{"state": {query.Get("state")}, "code": {"test-code"}, "client_id": {clientID}}.Encode()
	response, err := http.Get(callback.String()) // #nosec G107 -- test fixture's random loopback callback only.
	if err != nil {
		t.Fatal(err)
	}
	page, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	status := m.Status(t.Context())
	expected := "ChatGPT connected"
	if status.State != "connected" {
		expected = "Sign-in did not complete"
	}
	if readErr != nil || !strings.Contains(string(page), expected) {
		t.Fatalf("callback success page was not delivered: %v", readErr)
	}
	return status
}

func TestRegistrationSeparatesAccountsValidatesStateAndKeepsHostID(t *testing.T) {
	f := newOAuthFixture(t)
	m := fixtureManager(t, f, t.TempDir())
	callback, query := beginFixtureSignIn(t, m, f, "")
	callback.RawQuery = url.Values{"state": {"wrong"}, "code": {"test-code"}, "client_id": {"issued-one"}}.Encode()
	response, err := http.Get(callback.String()) // #nosec G107 -- local test callback.
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 400 || m.Status(t.Context()).Active != "" {
		t.Fatal("wrong state accepted")
	}
	first := finishFixtureSignIn(t, m, callback, query, "issued-one")
	if first.Active == "" || len(first.Profiles) != 1 || !first.Profiles[0].PlanAuthorized {
		t.Fatalf("sign-in not activated: %+v", first)
	}
	callback2, query2 := beginFixtureSignIn(t, m, f, "")
	if query.Get("ext_agent_host_id") != query2.Get("ext_agent_host_id") || query.Get("state") == query2.Get("state") || query.Get("nonce") == query2.Get("nonce") {
		t.Fatal("host/attempt identity not separated")
	}
	second := finishFixtureSignIn(t, m, callback2, query2, "issued-two")
	if len(second.Profiles) != 2 || second.Profiles[0].ID == second.Profiles[1].ID || second.Profiles[0].Label == second.Profiles[1].Label {
		t.Fatal("same-email workspace registrations were combined")
	}
	if err := m.AcknowledgeWelcome(t.Context()); err != nil {
		t.Fatal(err)
	}
	returnCallback, returnQuery := beginFixtureSignIn(t, m, f, second.Active)
	if returnQuery.Get("client_id") != "issued-two" || returnQuery.Get("agent_name_hint") != "" {
		t.Fatal("returning sign-in re-registered the client")
	}
	returnStatus := finishFixtureSignIn(t, m, returnCallback, returnQuery, "")
	if returnStatus.Profiles[1].WelcomePending {
		t.Fatal("welcome was shown again")
	}
	encoded, _ := json.Marshal(returnStatus)
	for _, secret := range []string{"private-access", "private-refresh", "issued-one", "issued-two", "eyJ"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("credential leaked in public status")
		}
	}
}

func TestInvalidIdentityClaimsNeverReplaceActiveRegistration(t *testing.T) {
	f := newOAuthFixture(t)
	m := fixtureManager(t, f, t.TempDir())
	callback, query := beginFixtureSignIn(t, m, f, "")
	active := finishFixtureSignIn(t, m, callback, query, "issued-original")
	cases := map[string]func(map[string]any){
		"issuer":     func(c map[string]any) { c["iss"] = "https://other.invalid" },
		"audience":   func(c map[string]any) { c["aud"] = "different-client" },
		"nonce":      func(c map[string]any) { c["nonce"] = "different-nonce" },
		"expired":    func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"future_iat": func(c map[string]any) { c["iat"] = time.Now().Add(time.Hour).Unix() },
		"subject":    func(c map[string]any) { c["sub"] = "different-user" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f.mu.Lock()
			f.claims = mutate
			f.mu.Unlock()
			callback, query := beginFixtureSignIn(t, m, f, active.Active)
			status := finishFixtureSignIn(t, m, callback, query, "")
			if status.State != "error" || status.Active != active.Active || len(status.Profiles) != 1 {
				t.Fatal("invalid identity replaced credentials")
			}
		})
	}
}

func TestRefreshIsSerializedAcrossManagersAndRotatesTogether(t *testing.T) {
	f := newOAuthFixture(t)
	directory := t.TempDir()
	m := fixtureManager(t, f, directory)
	callback, query := beginFixtureSignIn(t, m, f, "")
	_ = finishFixtureSignIn(t, m, callback, query, "issued-refresh")
	m.mu.Lock()
	err := m.transaction(t.Context(), func(data *credentialFile) error {
		data.Profiles[0].ExpiresAt = time.Now().Add(-time.Minute)
		return nil
	})
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	other := fixtureManager(t, f, directory)
	sources := make([]func(context.Context) (string, error), 0, 2)
	for _, manager := range []*Manager{m, other} {
		source, err := manager.ActiveTokenSource(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
	}
	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := source(t.Context())
			if err != nil || token != "renewed-access" {
				t.Error("refresh did not return replaced token")
			}
		}()
	}
	wg.Wait()
	if f.refreshes.Load() != 1 {
		t.Fatalf("rotating refresh raced: %d calls", f.refreshes.Load())
	}
	private, err := os.ReadFile(m.path)
	if err != nil || !strings.Contains(string(private), "rotated-refresh") || strings.Contains(string(private), "private-refresh") {
		t.Fatal("refresh pair not replaced atomically")
	}
}

func TestDeniedPlanScopeAndLogoutCannotSupplyCredentials(t *testing.T) {
	f := newOAuthFixture(t)
	f.scope = "openid profile email"
	m := fixtureManager(t, f, t.TempDir())
	callback, query := beginFixtureSignIn(t, m, f, "")
	status := finishFixtureSignIn(t, m, callback, query, "identity-only")
	source, err := m.ActiveTokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if token, err := source(t.Context()); err == nil || token != "" {
		t.Fatal("identity grant authorized plan inference")
	}
	f.revocationFails = true
	confirmed, err := m.Disconnect(t.Context(), status.Active)
	if err != nil || confirmed {
		t.Fatal("unconfirmed revocation reported as success")
	}
	if token, err := source(t.Context()); err == nil || token != "" {
		t.Fatal("captured token source survived logout")
	}
	status = m.Status(t.Context())
	if status.Active != "" || status.Profiles[0].Connected || status.Message == "" {
		t.Fatal("local logout did not clear credentials and explain revocation")
	}
	_, query2 := beginFixtureSignIn(t, m, f, status.Profiles[0].ID)
	if query.Get("ext_agent_host_id") != query2.Get("ext_agent_host_id") || query2.Get("client_id") != "identity-only" {
		t.Fatal("logout discarded host or registration")
	}
	m.Cancel()
}
