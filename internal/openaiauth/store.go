// Package openaiauth owns host-only OpenAI registrations and credentials.
// ChatGPT identity is never a MagicHandy account or controller permission.
package openaiauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const issuerURL = "https://auth.openai.com"
const resourceURL = "https://api.openai.com/v1"

// Options permits isolated protocol testing without substituting credentials.
type Options struct {
	DataDir string
	// LazyStorage defers credential I/O and platform security libraries until cloud use.
	LazyStorage bool
	Client      *http.Client
	Issuer      string
	OnChange    func()
}

type registration struct {
	Issuer              string    `json:"issuer"`
	WelcomeAcknowledged bool      `json:"welcome_acknowledged"`
	ID                  string    `json:"id"`
	ClientID            string    `json:"client_id"`
	Subject             string    `json:"subject"`
	Email               string    `json:"email"`
	IDToken             string    `json:"id_token,omitempty"`
	AccessToken         string    `json:"access_token,omitempty"`
	RefreshToken        string    `json:"refresh_token,omitempty"`
	Scopes              []string  `json:"scopes,omitempty"`
	ExpiresAt           time.Time `json:"expires_at"`
	EarliestRefreshAt   time.Time `json:"earliest_refresh_at"`
}

type credentialFile struct {
	ConnectionKeys map[string]connectionKey `json:"connection_keys,omitempty"`
	HostID         string                   `json:"ext_agent_host_id"`
	Active         string                   `json:"active"`
	Profiles       []registration           `json:"profiles"`
	APIKey         string                   `json:"decisions_api_key,omitempty"`
	// Retain issued IDs even if first exchange fails; codes themselves are never
	// persisted. These cannot become active until identity validation succeeds.
	PendingClients []string `json:"pending_client_ids,omitempty"`
}

// Profile is a browser-safe account label, with no token or client ID.
type Profile struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Connected      bool   `json:"connected"`
	PlanAuthorized bool   `json:"plan_authorized"`
	WelcomePending bool   `json:"welcome_pending"`
}

// Status contains backend-owned connection state. Readiness requires a real
// generation, and is deliberately reset when an account changes or disconnects.
type Status struct {
	Profiles        []Profile `json:"profiles"`
	Active          string    `json:"active"`
	Pending         bool      `json:"pending"`
	State           string    `json:"state"`
	Message         string    `json:"message,omitempty"`
	DecisionsKeySet bool      `json:"decisions_key_set"`
	Generation      uint64    `json:"generation"`
}

// Manager serializes rotating refresh across goroutines and processes.
type Manager struct {
	mu           sync.Mutex
	path         string
	issuer       string
	client       *http.Client
	pending      *signInAttempt
	generation   uint64
	state        string
	message      string
	closed       bool
	wg           sync.WaitGroup
	onChange     func()
	storageReady bool
}

// Open creates protected host storage without contacting an account service.
func Open(options Options) (*Manager, error) {
	directory := filepath.Join(options.DataDir, "openai-private")
	if options.Issuer == "" {
		options.Issuer = issuerURL
	}
	if options.Client == nil {
		options.Client = &http.Client{Timeout: 20 * time.Second}
	}
	// A redirect must never carry a code, refresh token, or revocation token
	// away from the validated discovery endpoint, including on the same host.
	client := *options.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	options.Client = &client
	m := &Manager{path: filepath.Join(directory, "credentials.json"), issuer: options.Issuer, client: options.Client, state: "disconnected", onChange: options.OnChange}
	if options.LazyStorage {
		return m, nil
	}
	m.mu.Lock()
	err := m.transaction(context.Background(), func(*credentialFile) error { return nil })
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func randomValue() string {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic("operating system random source failed")
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:])
}

func newHostID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", errors.New("create ChatGPT host identifier")
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
}

// transaction is called with mu held. The process lock protects the entire
// refresh and its atomic replacement so other app processes see the new token.
func (m *Manager) transaction(ctx context.Context, action func(*credentialFile) error) error {
	if !m.storageReady {
		directory := filepath.Dir(m.path)
		if err := os.MkdirAll(directory, 0700); err != nil {
			return errors.New("create protected OpenAI storage")
		}
		if err := restrictPath(directory); err != nil {
			return errors.New("protect OpenAI storage")
		}
		m.storageReady = true
	}
	unlock, err := lockCredentials(ctx, m.path+".lock")
	if err != nil {
		return errors.New("OpenAI credentials are busy or unavailable")
	}
	defer unlock()
	data := credentialFile{Profiles: []registration{}}
	raw, err := os.ReadFile(m.path)
	if err == nil {
		if len(raw) > 1<<20 || json.Unmarshal(raw, &data) != nil {
			return errors.New("OpenAI credentials could not be read; reconnect from settings")
		}
	} else if !os.IsNotExist(err) {
		return errors.New("OpenAI credentials could not be read")
	}

	if err := initializeCredentialHost(&data); err != nil {
		return err
	}
	if err := action(&data); err != nil {
		return err
	}
	encoded, err := json.Marshal(data) // #nosec G117 -- credentials are written only to owner-restricted host storage, never logs or public JSON.
	if err != nil {
		return errors.New("OpenAI credentials could not be saved")
	}
	if string(raw) == string(encoded) {
		return nil
	}
	temporary, err := os.CreateTemp(filepath.Dir(m.path), "credentials-*.tmp")
	if err != nil {
		return errors.New("OpenAI credentials could not be saved")
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	if err := restrictPath(temporary.Name()); err != nil {
		_ = temporary.Close()
		return errors.New("OpenAI credentials could not be protected")
	}
	_, writeErr := temporary.Write(encoded)
	if writeErr == nil {
		writeErr = temporary.Sync()
	}
	closeErr := temporary.Close()
	if writeErr != nil || closeErr != nil || os.Rename(temporary.Name(), m.path) != nil {
		return errors.New("OpenAI credentials could not be saved")
	}
	return nil
}

func initializeCredentialHost(data *credentialFile) error {
	if data.HostID == "" || (strings.HasPrefix(data.HostID, "magichandy-") && len(data.Profiles) == 0) {
		// Repair only a never-registered preview host; retain issued identities.
		hostID, err := newHostID()
		if err != nil {
			return err
		}
		data.HostID = hostID
	}
	return nil
}

// Status returns redacted connection state, never tokens or issued client IDs.
func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := Status{Profiles: []Profile{}, Pending: m.pending != nil, State: m.state, Message: m.message, Generation: m.generation}
	if !m.storageReady {
		if _, err := os.Stat(m.path); os.IsNotExist(err) {
			return status
		}
	}
	err := m.transaction(ctx, func(data *credentialFile) error {
		status.Active, status.DecisionsKeySet = data.Active, data.APIKey != ""
		for _, profile := range data.Profiles {
			label := profile.Email
			if label == "" {
				label = "ChatGPT account"
			}
			label += " · " + profile.ID[:min(6, len(profile.ID))]
			authorized := slices.Contains(profile.Scopes, "chatgpt.tokens.use.direct")
			status.Profiles = append(status.Profiles, Profile{ID: profile.ID, Label: label, Connected: profile.AccessToken != "", PlanAuthorized: authorized, WelcomePending: authorized && !profile.WelcomeAcknowledged})
		}
		return nil
	})
	if err != nil {
		status.State, status.Message = "unavailable", err.Error()
	}
	if status.State == "disconnected" && status.Active != "" {
		status.State = "connected"
	}
	return status
}

// ActiveTokenSource captures the selected registration and process generation.
// A result using an account superseded during sign-in/switch/logout is stale.
func (m *Manager) ActiveTokenSource(ctx context.Context) (llm.TokenSource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var id string
	err := m.transaction(ctx, func(data *credentialFile) error { id = data.Active; return nil })
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, &llm.CloudError{Kind: "signed_out"}
	}
	generation := m.generation
	return func(ctx context.Context) (string, error) { return m.accessToken(ctx, id, generation) }, nil
}

func (m *Manager) accessToken(ctx context.Context, id string, generation uint64) (string, error) {
	m.mu.Lock()
	changed := false
	defer func() {
		m.mu.Unlock()
		if changed {
			m.notifyChange()
		}
	}()
	if m.closed || generation != m.generation {
		return "", context.Canceled
	}
	var token string
	var outcome error
	err := m.transaction(ctx, func(data *credentialFile) error {
		if data.Active != id {
			return context.Canceled
		}
		index := slices.IndexFunc(data.Profiles, func(p registration) bool { return p.ID == id })
		if index < 0 || data.Profiles[index].AccessToken == "" {
			return &llm.CloudError{Kind: "signed_out"}
		}
		profile := &data.Profiles[index]
		if !slices.Contains(profile.Scopes, "chatgpt.tokens.use.direct") {
			return &llm.CloudError{Kind: "permission"}
		}
		if time.Now().Add(90 * time.Second).After(profile.ExpiresAt) {
			if time.Now().Before(profile.EarliestRefreshAt) {
				return &llm.CloudError{Kind: "unavailable"}
			}
			if err := m.refresh(ctx, profile); err != nil {
				var failure *llm.CloudError
				if errors.As(err, &failure) && failure.Kind == "signed_out" {
					profile.AccessToken, profile.RefreshToken, profile.IDToken = "", "", ""
					m.generation++
					changed = true
					outcome = err
					return nil
				}
				return err
			}
		}
		if !slices.Contains(profile.Scopes, "chatgpt.tokens.use.direct") {
			m.generation++
			changed = true
			outcome = &llm.CloudError{Kind: "permission"}
			return nil
		}
		token = profile.AccessToken
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, outcome
}

// APIKeySource returns only the separately authorized Decisions credential.
func (m *Manager) APIKeySource(ctx context.Context) (llm.TokenSource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var configured bool
	err := m.transaction(ctx, func(data *credentialFile) error { configured = data.APIKey != ""; return nil })
	if err != nil {
		return nil, err
	}
	if !configured {
		return nil, &llm.CloudError{Kind: "signed_out"}
	}
	generation := m.generation
	return func(ctx context.Context) (string, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed || generation != m.generation {
			return "", context.Canceled
		}
		var key string
		err := m.transaction(ctx, func(data *credentialFile) error { key = data.APIKey; return nil })
		if err == nil && key == "" {
			err = &llm.CloudError{Kind: "signed_out"}
		}
		return key, err
	}, nil
}

// SetAPIKey saves or removes the separate Decisions billing credential.
func (m *Manager) SetAPIKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if len(key) > 512 || strings.ContainsAny(key, "\r\n\t ") {
		return errors.New("invalid OpenAI API key")
	}
	m.mu.Lock()
	err := m.transaction(ctx, func(data *credentialFile) error { data.APIKey = key; return nil })
	if err == nil {
		m.generation++
	}
	m.mu.Unlock()
	if err == nil {
		m.notifyChange()
	}
	return err
}

// Select activates one verified account/workspace registration.
func (m *Manager) Select(ctx context.Context, id string) error {
	m.mu.Lock()
	err := m.transaction(ctx, func(data *credentialFile) error {
		if !slices.ContainsFunc(data.Profiles, func(p registration) bool { return p.ID == id && p.AccessToken != "" }) {
			return errors.New("connect this ChatGPT account first")
		}
		data.Active = id
		return nil
	})
	if err == nil {
		m.generation++
		m.state = "connected"
		m.message = ""
	}
	m.mu.Unlock()
	if err == nil {
		m.notifyChange()
	}
	return err
}

// AcknowledgeWelcome records the plan-usage notice for this registration once.
func (m *Manager) AcknowledgeWelcome(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transaction(ctx, func(data *credentialFile) error {
		for index := range data.Profiles {
			if data.Profiles[index].ID == data.Active {
				data.Profiles[index].WelcomeAcknowledged = true
			}
		}
		return nil
	})
}

// Close cancels sign-in callbacks and waits for their listener teardown.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.generation++
	if m.pending != nil {
		m.pending.cancel()
		m.pending = nil
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) notifyChange() {
	if m.onChange != nil {
		m.onChange()
	}
}

// Generation identifies account/key changes without reading or exposing secrets.
func (m *Manager) Generation() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generation
}
