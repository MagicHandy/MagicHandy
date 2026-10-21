package openaiauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestConnectionKeysNeverCrossProvidersEndpointsOrGenerations(t *testing.T) {
	m, err := Open(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	connection := config.ModelConnection{ID: "connection", Name: "Connection", Provider: config.LLMProviderCompatible, BaseURL: "https://example.test/v1", Model: "model"}.Normalize()
	if err := m.SetConnectionKey(t.Context(), connection, "own-key"); err != nil {
		t.Fatal(err)
	}
	source := m.ConnectionKeySource(connection)
	if token, err := source(t.Context()); err != nil || token != "own-key" {
		t.Fatal("bound credential unavailable")
	}
	for _, mutated := range []config.ModelConnection{
		{ID: connection.ID, Name: connection.Name, Provider: config.LLMProviderOpenRouter, Model: connection.Model},
		{ID: connection.ID, Name: connection.Name, Provider: connection.Provider, BaseURL: "https://example.test/other", Model: connection.Model},
		{ID: "other", Name: connection.Name, Provider: connection.Provider, BaseURL: connection.BaseURL, Model: connection.Model},
	} {
		if token, err := m.ConnectionKeySource(mutated)(t.Context()); err == nil || token != "" {
			t.Fatal("credential crossed connection ownership")
		}
	}
	if token, err := m.APIKeySource(t.Context()); err == nil || token != nil {
		t.Fatal("provider key authorized Decisions billing")
	}
	if err := m.SetConnectionKey(t.Context(), connection, "replacement"); err != nil {
		t.Fatal(err)
	}
	if token, err := source(t.Context()); !errors.Is(err, context.Canceled) || token != "" {
		t.Fatal("old token source survived key replacement")
	}
	public, _ := json.Marshal(m.Status(t.Context()))
	if strings.Contains(string(public), "replacement") || strings.Contains(string(public), "own-key") {
		t.Fatal("credential leaked through status")
	}
}

func TestOAuthTokenRedirectNeverReceivesRefreshOrAuthorizationCode(t *testing.T) {
	var leaked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sink" {
			leaked.Store(true)
			return
		}
		http.Redirect(w, r, "/sink", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	m, err := Open(Options{DataDir: t.TempDir(), Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	_, err = m.exchange(t.Context(), server.URL+"/token", map[string][]string{"refresh_token": {"fixture-only-token"}})
	if err == nil || leaked.Load() {
		t.Fatal("OAuth credential followed redirect")
	}
}

func TestFailedFirstExchangeReusesIssuedRegistrationAfterRestart(t *testing.T) {
	f := newOAuthFixture(t)
	f.claims = func(claims map[string]any) { claims["nonce"] = "invalid" }
	directory := t.TempDir()
	m := fixtureManager(t, f, directory)
	callback, query := beginFixtureSignIn(t, m, f, "")
	failed := finishFixtureSignIn(t, m, callback, query, "issued-before-failure")
	if failed.Active != "" || len(failed.Profiles) != 1 || failed.Profiles[0].Connected {
		t.Fatal("unverified registration became connected")
	}
	other := fixtureManager(t, f, directory)
	f.mu.Lock()
	f.claims = nil
	f.mu.Unlock()
	callback, query = beginFixtureSignIn(t, other, f, failed.Profiles[0].ID)
	if query.Get("client_id") != "issued-before-failure" || query.Get("agent_name_hint") != "" {
		t.Fatal("retry created a second dynamic registration")
	}
	connected := finishFixtureSignIn(t, other, callback, query, "")
	if connected.Active != failed.Profiles[0].ID || len(connected.Profiles) != 1 || !connected.Profiles[0].Connected {
		t.Fatal("verified retry lost registration mapping")
	}
}

func TestRefreshScopeLossPersistsRotatedPairWithoutInference(t *testing.T) {
	f := newOAuthFixture(t)
	f.refreshScope = "openid profile email"
	m := fixtureManager(t, f, t.TempDir())
	callback, query := beginFixtureSignIn(t, m, f, "")
	_ = finishFixtureSignIn(t, m, callback, query, "scope-loss")
	m.mu.Lock()
	err := m.transaction(t.Context(), func(data *credentialFile) error {
		data.Profiles[0].ExpiresAt = time.Now().Add(-time.Minute)
		return nil
	})
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	source, err := m.ActiveTokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	generation := m.Status(t.Context()).Generation
	token, err := source(t.Context())
	var outcome *llm.CloudError
	if token != "" || !errors.As(err, &outcome) || outcome.Kind != "permission" {
		t.Fatal("lost scope still authorized inference")
	}
	if m.Status(t.Context()).Profiles[0].PlanAuthorized {
		t.Fatal("scope loss was not persisted")
	}
	if m.Status(t.Context()).Generation <= generation {
		t.Fatal("scope loss left old readiness proofs valid")
	}
	if _, err := source(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatalf("old token source survived scope loss: %v", err)
	}
}

func TestRemoveConnectionKeysDeletesOnlyNamedConnections(t *testing.T) {
	m, err := Open(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	removed := config.ModelConnection{ID: "openrouter-1", Name: "OpenRouter", Provider: config.LLMProviderOpenRouter, Model: "model"}.Normalize()
	kept := config.ModelConnection{ID: "openrouter-2", Name: "OpenRouter", Provider: config.LLMProviderOpenRouter, Model: "model"}.Normalize()
	for _, connection := range []config.ModelConnection{removed, kept} {
		if err := m.SetConnectionKey(t.Context(), connection, "key-"+connection.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.RemoveConnectionKeys(t.Context(), []string{removed.ID, "never-saved"}); err != nil {
		t.Fatal(err)
	}
	if m.ConnectionKeySet(t.Context(), removed) {
		t.Fatal("removed connection kept its key")
	}
	if !m.ConnectionKeySet(t.Context(), kept) {
		t.Fatal("unrelated connection lost its key")
	}
	// A new connection that reuses the id starts without a credential.
	if token, err := m.ConnectionKeySource(removed)(t.Context()); err == nil || token != "" {
		t.Fatal("reused id inherited a removed key")
	}
}
