package openaiauth

import (
	"context"
	"errors"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type connectionKey struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Key      string `json:"key"`
}

// SetConnectionKey binds a secret to the explicit provider and full endpoint.
// Editing a connection never repurposes another provider's credential.
func (m *Manager) SetConnectionKey(ctx context.Context, connection config.ModelConnection, key string) error {
	connection = connection.Normalize()
	if err := connection.Validate(); err != nil {
		return err
	}
	if connection.Provider == config.LLMProviderChatGPT {
		return errors.New("ChatGPT connections use sign-in, not an API key")
	}
	key = strings.TrimSpace(key)
	if len(key) > 512 || strings.ContainsAny(key, "\r\n\t ") {
		return errors.New("invalid model API key")
	}
	m.mu.Lock()
	err := m.transaction(ctx, func(data *credentialFile) error {
		if data.ConnectionKeys == nil {
			data.ConnectionKeys = map[string]connectionKey{}
		}
		if key == "" {
			delete(data.ConnectionKeys, connection.ID)
		} else {
			data.ConnectionKeys[connection.ID] = connectionKey{Provider: connection.Provider, BaseURL: connection.BaseURL, Key: key}
		}
		return nil
	})
	if err == nil {
		m.generation++
	}
	m.mu.Unlock()
	if err == nil {
		m.notifyChange()
	}
	return err
}

// ConnectionKeySet reports whether an exact endpoint binding has a credential.
func (m *Manager) ConnectionKeySet(ctx context.Context, connection config.ModelConnection) bool {
	connection = connection.Normalize()
	m.mu.Lock()
	defer m.mu.Unlock()
	set := false
	_ = m.transaction(ctx, func(data *credentialFile) error {
		key := data.ConnectionKeys[connection.ID]
		set = key.Key != "" && key.Provider == connection.Provider && key.BaseURL == connection.BaseURL
		return nil
	})
	return set
}

// ConnectionKeySource revalidates credential ownership on every request.
func (m *Manager) ConnectionKeySource(connection config.ModelConnection) llm.TokenSource {
	connection = connection.Normalize()
	m.mu.Lock()
	generation := m.generation
	m.mu.Unlock()
	return func(ctx context.Context) (string, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed || generation != m.generation {
			return "", context.Canceled
		}
		var token string
		err := m.transaction(ctx, func(data *credentialFile) error {
			key := data.ConnectionKeys[connection.ID]
			if key.Key == "" || key.Provider != connection.Provider || key.BaseURL != connection.BaseURL {
				return &llm.CloudError{Kind: "signed_out"}
			}
			token = key.Key
			return nil
		})
		return token, err
	}
}
