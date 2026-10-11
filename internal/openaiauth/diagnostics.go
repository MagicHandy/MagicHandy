package openaiauth

import (
	"context"
	"net/url"
	"os"
	"strings"
)

// RedactDiagnostics replaces stored credentials without returning them to a
// caller. Local-only failures leave absent cloud storage dormant.
func (m *Manager) RedactDiagnostics(ctx context.Context, values []string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.storageReady {
		if _, err := os.Stat(m.path); os.IsNotExist(err) {
			return values, nil
		}
	}
	var replacements []string
	err := m.transaction(ctx, func(data *credentialFile) error {
		add := func(secret string) {
			if secret != "" {
				for _, form := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
					replacements = append(replacements, form, "[redacted]")
				}
			}
		}
		add(data.APIKey)
		for _, key := range data.ConnectionKeys {
			add(key.Key)
		}
		for _, profile := range data.Profiles {
			add(profile.AccessToken)
			add(profile.RefreshToken)
			add(profile.IDToken)
			add(profile.ClientID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	replacer := strings.NewReplacer(replacements...)
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = replacer.Replace(value)
	}
	return result, nil
}
