package openaiauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
}

type tokenReply struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token"`
	IDToken           string `json:"id_token"`
	TokenType         string `json:"token_type"`
	ExpiresIn         int    `json:"expires_in"`
	Scope             string `json:"scope"`
	EarliestRefreshAt int64  `json:"earliest_refresh_at"`
}

type signInAttempt struct {
	returnURL                                             string
	state, nonce, verifier, callback, clientID, profileID string
	ctx                                                   context.Context
	cancel                                                context.CancelFunc
	discovery                                             discovery
	consumed                                              bool
}

// Start binds a short-lived loopback callback before returning an authorization
// URL. The caller opens it on the app host; remote browsers cannot complete it.
func (m *Manager) Start(ctx context.Context, profileID string, returnURLs ...string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", context.Canceled
	}
	if m.pending != nil {
		m.pending.cancel()
		m.pending = nil
	}
	discovery, err := m.discover(ctx)
	if err != nil {
		return "", err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", errors.New("the host could not open a ChatGPT sign-in callback")
	}
	attemptCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	attempt := &signInAttempt{state: randomValue(), nonce: randomValue(), verifier: randomValue(), callback: "http://" + listener.Addr().String() + "/auth/callback", clientID: "dynamic_agent_client", profileID: profileID, ctx: attemptCtx, cancel: cancel, discovery: discovery}
	if len(returnURLs) > 0 {
		target, err := url.Parse(returnURLs[0])
		if err == nil && (target.Scheme == "http" || target.Scheme == "https") && target.User == nil && (target.Hostname() == "localhost" || net.ParseIP(target.Hostname()).IsLoopback()) {
			attempt.returnURL = target.String()
		}
	}
	query := url.Values{"response_type": {"code"}, "redirect_uri": {attempt.callback}, "scope": {"openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"}, "resource": {resourceURL}, "state": {attempt.state}, "nonce": {attempt.nonce}, "code_challenge_method": {"S256"}}
	err = m.transaction(ctx, func(data *credentialFile) error {
		query.Set("ext_agent_host_id", data.HostID)
		if profileID != "" {
			index := slices.IndexFunc(data.Profiles, func(p registration) bool { return p.ID == profileID })
			if index < 0 {
				return errors.New("unknown ChatGPT account")
			}
			attempt.clientID = data.Profiles[index].ClientID
			query.Set("login_hint", data.Profiles[index].Email)
		} else {
			query.Set("agent_name_hint", "MagicHandy")
		}
		return nil
	})
	if err != nil {
		cancel()
		_ = listener.Close()
		return "", err
	}
	query.Set("client_id", attempt.clientID)
	hash := sha256.Sum256([]byte(attempt.verifier))
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(hash[:]))
	m.pending = attempt
	m.state = "signing_in"
	m.message = "Complete sign-in in the browser on this computer."
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/callback", func(w http.ResponseWriter, r *http.Request) { m.callback(w, r, attempt) })
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second}
	shutdownDone := make(chan struct{})
	stopClose := context.AfterFunc(attemptCtx, func() {
		defer close(shutdownDone)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
	})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		_ = server.Serve(listener)
		if !stopClose() {
			<-shutdownDone
		}
		cancel()
		m.mu.Lock()
		if m.pending == attempt {
			m.pending = nil
			m.state = "disconnected"
			m.message = "Sign-in expired or was canceled. Try again from settings."
		}
		m.mu.Unlock()
	}()
	return discovery.AuthorizationEndpoint + "?" + query.Encode(), nil
}

// Cancel tears down the pending sign-in while preserving verified accounts.
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending != nil {
		m.pending.cancel()
		m.pending = nil
	}
	m.state = "disconnected"
	m.message = ""
}

func (m *Manager) callback(w http.ResponseWriter, r *http.Request, attempt *signInAttempt) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	m.mu.Lock()
	if m.pending != attempt || attempt.consumed || attempt.ctx.Err() != nil || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(attempt.state)) != 1 {
		m.mu.Unlock()
		http.Error(w, "This sign-in attempt is invalid or expired.", http.StatusBadRequest)
		return
	}
	attempt.consumed = true
	err := m.completeCallback(attempt, r.URL.Query())
	m.pending = nil
	if err != nil {
		m.state = "error"
		m.message = err.Error()
	} else {
		m.state = "connected"
		m.message = ""
		m.generation++
	}
	m.mu.Unlock()
	if err == nil {
		m.notifyChange()
	}
	if err != nil {
		_, _ = io.WriteString(w, "<!doctype html><title>MagicHandy</title><h1>Sign-in did not complete</h1><p>Return to MagicHandy settings for details and try again.</p>")
	} else {
		_, _ = io.WriteString(w, "<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Connected to MagicHandy</title><h1>ChatGPT connected</h1><p>Sign-in completed. Return to MagicHandy to choose a model and test a text-only response.</p>")
		if attempt.returnURL != "" {
			_, _ = io.WriteString(w, "<p><a href=\""+html.EscapeString(attempt.returnURL)+"\">Return to MagicHandy</a></p>")
		}
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	attempt.cancel()
}

func (m *Manager) completeCallback(attempt *signInAttempt, query url.Values) error {
	if query.Get("error") != "" {
		return errors.New("sign-in with ChatGPT was declined; choose a model connection in settings")
	}
	clientID := query.Get("client_id")
	if attempt.clientID == "dynamic_agent_client" {
		if clientID == "" || clientID == "dynamic_agent_client" || len(clientID) > 256 {
			return errors.New("ChatGPT registration did not issue a client ID")
		}
		attempt.clientID = clientID
		if err := m.transaction(attempt.ctx, func(data *credentialFile) error {
			if !slices.Contains(data.PendingClients, clientID) {
				data.PendingClients = append(data.PendingClients, clientID)
			}
			if !slices.ContainsFunc(data.Profiles, func(profile registration) bool { return profile.ClientID == clientID }) {
				if len(data.Profiles) >= 32 {
					return errors.New("too many ChatGPT registrations on this host")
				}
				// Keep the issued client retryable, but disconnected until its
				// signed identity is verified. Never issue inference credentials.
				data.Profiles = append(data.Profiles, registration{ID: randomValue(), Issuer: m.issuer, ClientID: clientID})
			}
			return nil
		}); err != nil {
			return err
		}
	} else if clientID != "" && clientID != attempt.clientID {
		return errors.New("ChatGPT returned a different account registration")
	}
	code := query.Get("code")
	if code == "" || len(code) > 8192 {
		return errors.New("ChatGPT did not return an authorization code")
	}
	tokens, err := m.exchange(attempt.ctx, attempt.discovery.TokenEndpoint, url.Values{"grant_type": {"authorization_code"}, "client_id": {attempt.clientID}, "code": {code}, "code_verifier": {attempt.verifier}, "redirect_uri": {attempt.callback}, "resource": {resourceURL}})
	if err != nil {
		return err
	}
	identity, err := m.verifyIDToken(attempt.ctx, attempt.discovery, tokens.IDToken, attempt.clientID, attempt.nonce)
	if err != nil {
		return err
	}
	return m.saveVerifiedRegistration(attempt, identity, tokens)
}

func (m *Manager) saveVerifiedRegistration(attempt *signInAttempt, identity verifiedIdentity, tokens tokenReply) error {
	return m.transaction(attempt.ctx, func(data *credentialFile) error {
		index := slices.IndexFunc(data.Profiles, func(p registration) bool { return p.ClientID == attempt.clientID })
		if attempt.profileID != "" && (index < 0 || data.Profiles[index].ID != attempt.profileID) {
			return errors.New("ChatGPT identity does not match the selected registration")
		}
		profile := registration{ID: randomValue(), Issuer: m.issuer, ClientID: attempt.clientID, Subject: identity.Subject, Email: identity.Email}
		if index >= 0 {
			if data.Profiles[index].Subject != "" && data.Profiles[index].Subject != identity.Subject {
				return errors.New("ChatGPT registration identity changed")
			}
			profile.ID = data.Profiles[index].ID
			profile.WelcomeAcknowledged = data.Profiles[index].WelcomeAcknowledged
		}
		applyTokens(&profile, tokens, false)
		if index >= 0 {
			data.Profiles[index] = profile
		} else {
			data.Profiles = append(data.Profiles, profile)
		}
		data.Active = profile.ID
		data.PendingClients = slices.DeleteFunc(data.PendingClients, func(id string) bool { return id == attempt.clientID })
		return nil
	})
}

func (m *Manager) discover(ctx context.Context) (discovery, error) {
	var document discovery
	if err := m.getJSON(ctx, m.issuer+"/.well-known/openid-configuration", &document); err != nil {
		return document, err
	}
	if document.Issuer != m.issuer {
		return discovery{}, errors.New("OpenAI issuer discovery was invalid")
	}
	for _, endpoint := range []string{document.AuthorizationEndpoint, document.TokenEndpoint, document.JWKSURI, document.RevocationEndpoint} {
		parsed, err := url.Parse(endpoint)
		origin, _ := url.Parse(m.issuer)
		if err != nil || parsed.Scheme != origin.Scheme || parsed.Host != origin.Host || parsed.User != nil || parsed.Fragment != "" {
			return discovery{}, errors.New("OpenAI discovery endpoint was invalid")
		}
	}
	return document, nil
}

func (m *Manager) getJSON(ctx context.Context, endpoint string, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("OpenAI account service is unavailable")
	}
	response, err := m.client.Do(request)
	if err != nil {
		return errors.New("OpenAI account service is unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result) != nil {
		return errors.New("OpenAI account service returned an invalid response")
	}
	return nil
}

func (m *Manager) exchange(ctx context.Context, endpoint string, form url.Values) (tokenReply, error) {
	var tokens tokenReply
	response, err := m.postForm(ctx, endpoint, form)
	if err != nil {
		return tokens, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests {
			return tokens, &llm.CloudError{Kind: "unavailable"}
		}
		return tokens, &llm.CloudError{Kind: "signed_out"}
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokens) != nil || tokens.AccessToken == "" || !strings.EqualFold(tokens.TokenType, "Bearer") || tokens.ExpiresIn <= 0 || tokens.ExpiresIn > 86400 {
		return tokenReply{}, errors.New("OpenAI returned an invalid token response")
	}
	return tokens, nil
}

func (m *Manager) postForm(ctx context.Context, endpoint string, form url.Values) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.New("OpenAI account request could not be prepared")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := m.client.Do(request)
	if err != nil {
		return nil, errors.New("OpenAI account service is unavailable")
	}
	return response, nil
}

func applyTokens(profile *registration, tokens tokenReply, refresh bool) {
	profile.AccessToken = tokens.AccessToken
	profile.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	profile.EarliestRefreshAt = time.Unix(tokens.EarliestRefreshAt, 0)
	if tokens.RefreshToken != "" {
		profile.RefreshToken = tokens.RefreshToken
	}
	if tokens.IDToken != "" {
		profile.IDToken = tokens.IDToken
	}
	if tokens.Scope != "" || !refresh {
		profile.Scopes = strings.Fields(tokens.Scope)
	}
}

func (m *Manager) refresh(ctx context.Context, profile *registration) error {
	if profile.RefreshToken == "" {
		return &llm.CloudError{Kind: "signed_out"}
	}
	discovery, err := m.discover(ctx)
	if err != nil {
		return err
	}
	tokens, err := m.exchange(ctx, discovery.TokenEndpoint, url.Values{"grant_type": {"refresh_token"}, "client_id": {profile.ClientID}, "refresh_token": {profile.RefreshToken}, "resource": {resourceURL}})
	if err != nil {
		return err
	}
	if tokens.RefreshToken == "" {
		return errors.New("OpenAI did not rotate the renewable session; reconnect")
	}
	if tokens.IDToken != "" {
		identity, err := m.verifyIDToken(ctx, discovery, tokens.IDToken, profile.ClientID, "")
		if err != nil || identity.Subject != profile.Subject {
			return errors.New("OpenAI refreshed identity does not match this account")
		}
	}
	applyTokens(profile, tokens, true)
	return nil
}

// Disconnect attempts revocation, always clears tokens locally, and preserves
// the verified account/client mapping for a later sign-in.
func (m *Manager) Disconnect(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer func() { m.mu.Unlock(); m.notifyChange() }()
	m.generation++
	if m.pending != nil {
		m.pending.cancel()
		m.pending = nil
	}
	confirmed := true
	err := m.transaction(ctx, func(data *credentialFile) error {
		index := slices.IndexFunc(data.Profiles, func(p registration) bool { return p.ID == id })
		if index < 0 {
			return errors.New("unknown ChatGPT account")
		}
		profile := &data.Profiles[index]
		if profile.RefreshToken != "" {
			confirmed = m.revoke(ctx, *profile)
		}
		profile.AccessToken, profile.RefreshToken, profile.IDToken = "", "", ""
		profile.Scopes = nil
		if data.Active == id {
			data.Active = ""
		}
		return nil
	})
	m.state = "disconnected"
	m.message = ""
	if !confirmed {
		m.message = "Signed out locally. Remote revocation was not confirmed; disconnect MagicHandy in ChatGPT Settings."
	}
	return confirmed, err
}

func (m *Manager) revoke(ctx context.Context, profile registration) bool {
	discovery, err := m.discover(ctx)
	if err != nil {
		return false
	}
	for attempt := 0; attempt < 2; attempt++ {
		response, err := m.postForm(ctx, discovery.RevocationEndpoint, url.Values{"token": {profile.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {profile.ClientID}})
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return true
			}
			if response.StatusCode < 500 {
				return false
			}
		}
		if attempt == 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	return false
}
