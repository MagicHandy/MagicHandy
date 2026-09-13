package accounts

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPassiveSessionChecksDoNotExtendIdleLifetime(t *testing.T) {
	store, _ := newAccountStore(t)
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	admin, err := store.BootstrapAdmin(t.Context(), "owner", "a long review passphrase")
	if err != nil {
		t.Fatal(err)
	}
	token, session, err := store.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	for range 29 {
		now = now.Add(time.Minute)
		if _, err := store.InspectSession(t.Context(), token); err != nil {
			t.Fatalf("passive inspection: %v", err)
		}
		if _, err := store.CheckSession(t.Context(), session.Key); err != nil {
			t.Fatalf("watchdog inspection: %v", err)
		}
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.InspectSession(t.Context(), token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("idle session survived polling: %v", err)
	}
}

func TestSessionKeyIsPrivateAndCannotAuthenticateAsBearer(t *testing.T) {
	store, _ := newAccountStore(t)
	admin, err := store.BootstrapAdmin(t.Context(), "owner", "a long review passphrase")
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := store.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if session.Key == "" || strings.Contains(string(data), session.Key) {
		t.Fatal("private session key missing or serialized")
	}
	if _, err := store.ResolveSession(t.Context(), session.Key); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("database key accepted as bearer credential: %v", err)
	}
}
