package openaiauth

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestLegacyCredentialsMigrateWithoutLosingBindingsOrRewritingOnRead(t *testing.T) {
	directory := t.TempDir()
	m, err := Open(Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	legacy := credentialFile{HostID: "urn:uuid:existing-host", Revision: 9, Active: "account", APIKey: "fixture-decisions-key", PendingClients: []string{"issued-pending-client"},
		Profiles:       []registration{{ID: "account", ClientID: "issued-client", AccessToken: "fixture-access-token", RefreshToken: "fixture-refresh-token", IDToken: "fixture-id-token", Scopes: []string{"chatgpt.tokens.use.direct"}, ExpiresAt: time.Now().Add(time.Hour).UTC(), WelcomeAcknowledged: true}},
		ConnectionKeys: map[string]connectionKey{"router": {Provider: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Key: "fixture-router-key"}},
	}
	plain, err := json.Marshal(legacy) // #nosec G117 -- synthetic migration fixture, no live credentials.
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.path, plain, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	data, decoded, _, err := readCredentials(m.path)
	defer clear(decoded)
	if err != nil || !reflect.DeepEqual(data, legacy) {
		t.Fatal("migration changed credential ownership or tokens", err)
	}
	before, err := os.ReadFile(m.path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if !bytes.HasPrefix(before, []byte(sealedCredentialHeader)) {
			t.Fatal("Windows migration did not seal credentials")
		}
		for _, secret := range []string{"fixture-access-token", "fixture-refresh-token", "fixture-id-token", "fixture-router-key", "fixture-decisions-key", "issued-client"} {
			if bytes.Contains(before, []byte(secret)) {
				t.Fatal("plaintext credential survived migration")
			}
		}
	}
	if status := reopened.Status(t.Context()); status.Active != "account" || !status.DecisionsKeySet || status.RecoveryRequired {
		t.Fatal("migrated status unavailable")
	}
	after, err := os.ReadFile(m.path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("read-only status rewrote stored ciphertext")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(m.path), "*.tmp"))
	if err != nil || len(files) != 0 {
		t.Fatal("migration retained temporary files")
	}
}

func TestUnreadableCredentialsStayIntactUntilExplicitRecovery(t *testing.T) {
	for _, broken := range []string{sealedCredentialHeader + "unusable-dpapi-bytes", "MAGICHANDY-SEALED-CREDENTIALS\nv9:unknown\npayload", "{broken-json", "null"} {
		t.Run(broken[:min(12, len(broken))], func(t *testing.T) {
			directory := t.TempDir()
			m, err := Open(Options{DataDir: directory})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(m.Close)
			if err := m.SetAPIKey(t.Context(), "fixture-retired-key"); err != nil {
				t.Fatal(err)
			}
			source, err := m.APIKeySource(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(m.path, []byte(broken), 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(Options{DataDir: directory})
			if err != nil {
				t.Fatal("unreadable store prevented app startup", err)
			}
			t.Cleanup(reopened.Close)
			status := reopened.Status(t.Context())
			if !status.RecoveryRequired || status.State != "unavailable" || status.DecisionsKeySet || len(status.Profiles) != 0 {
				t.Fatal("recovery state misrepresented")
			}
			if err := m.SetAPIKey(t.Context(), "replacement"); err == nil {
				t.Fatal("ordinary key save silently reset unreadable credentials")
			}
			retained, err := os.ReadFile(m.path)
			if err != nil || string(retained) != broken {
				t.Fatal("unreadable credentials changed without confirmation")
			}
			generation := m.Generation()
			if err := m.ResetUnreadableCredentials(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertRecoveredCredentials(t, m, generation, source)
		})
	}
}

func assertRecoveredCredentials(t *testing.T, m *Manager, generation uint64, source func(context.Context) (string, error)) {
	t.Helper()
	if m.Generation() == generation {
		t.Fatal("recovery did not retire requests")
	}
	if value, err := source(t.Context()); err == nil || value != "" {
		t.Fatal("old source survived credential reset")
	}
	if status := m.Status(t.Context()); status.RecoveryRequired || status.DecisionsKeySet || status.Active != "" {
		t.Fatal("recovered store not empty and usable")
	}
	if err := m.SetAPIKey(t.Context(), "fixture-new-key"); err != nil {
		t.Fatal("reconnect failed", err)
	}
	if err := m.ResetUnreadableCredentials(t.Context()); err == nil {
		t.Fatal("stale reset erased readable credentials")
	}
	if !m.Status(t.Context()).DecisionsKeySet {
		t.Fatal("stale reset removed a new key")
	}
}
