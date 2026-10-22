package accounts

import (
	"errors"
	"testing"
	"time"
)

func TestSessionIdlePolicyExtendsLiveLoginsWithoutRevivingExpiredOnes(t *testing.T) {
	store, db := newAccountStore(t)
	now := time.Now().UTC()
	store.now = func() time.Time { return now }
	admin := managementAccount(t, store, "owner", RoleAdmin)
	_, actor := managementSession(t, store, admin.ID, SessionClient{})
	if _, err := store.SetSessionPolicy(t.Context(), actor.Key, 30); err != nil {
		t.Fatal(err)
	}
	oldToken, _ := managementSession(t, store, admin.ID, SessionClient{})
	now = now.Add(20 * time.Minute)
	liveToken, _ := managementSession(t, store, admin.ID, SessionClient{})
	now = now.Add(11 * time.Minute)
	_, actor = managementSession(t, store, admin.ID, SessionClient{})
	if _, err := store.SetSessionPolicy(t.Context(), actor.Key, 480); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if _, err := store.InspectSession(t.Context(), oldToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revived expired login: %v", err)
	}
	if _, err := store.InspectSession(t.Context(), liveToken); err != nil {
		t.Fatalf("live login not extended: %v", err)
	}
	// A second store sees the durable policy without depending on a cache.
	other, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := other.SessionPolicy(t.Context())
	if err != nil || policy.IdleMinutes != 480 || policy.LifetimeMinutes != 720 {
		t.Fatalf("policy=%+v err=%v", policy, err)
	}
}

func TestSessionIdleDisabledStillEnforcesAbsoluteExpiryAndRevocation(t *testing.T) {
	store, _ := newAccountStore(t)
	now := time.Now().UTC()
	store.now = func() time.Time { return now }
	admin := managementAccount(t, store, "owner", RoleAdmin)
	token, actor := managementSession(t, store, admin.ID, SessionClient{})
	if _, err := store.SetSessionPolicy(t.Context(), actor.Key, 0); err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Hour)
	if _, err := store.CheckSession(t.Context(), actor.Key); err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ListOwnSessions(t.Context(), actor.Key)
	if err != nil || len(sessions) != 1 || sessions[0].IdleExpiresAt != nil {
		t.Fatalf("off policy projection: %+v %v", sessions, err)
	}
	if err := store.RenameOwnSession(t.Context(), actor.Key, actor.ID, "Review browser"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoveryStatus(t.Context(), actor.Key); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if _, err := store.InspectSession(t.Context(), token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("absolute expiry disabled")
	}
	if _, err := store.RecoveryStatus(t.Context(), actor.Key); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("recovery read accepted expired login")
	}
	if err := store.RenameOwnSession(t.Context(), actor.Key, actor.ID, "Expired"); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("mutation accepted expired login")
	}
	token, _ = managementSession(t, store, admin.ID, SessionClient{})
	if err := store.RevokeSession(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveSession(t.Context(), token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("revoked login survived")
	}
}

func TestIdlePolicyUpgradeRetiresLegacyExpiredSessions(t *testing.T) {
	store, db := newAccountStore(t)
	admin := managementAccount(t, store, "owner", RoleAdmin)
	oldToken, old := managementSession(t, store, admin.ID, SessionClient{})
	liveToken, _ := managementSession(t, store, admin.ID, SessionClient{})
	if _, err := db.SQL().Exec(`UPDATE user_sessions SET last_seen_at=? WHERE token_hash=?`, time.Now().Add(-31*time.Minute).UTC().Format(time.RFC3339Nano), old.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`DELETE FROM app_kv WHERE key=?`, sessionIdleKey); err != nil {
		t.Fatal(err)
	}
	upgraded, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := upgraded.InspectSession(t.Context(), oldToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("upgrade revived an expired login")
	}
	if _, err := upgraded.InspectSession(t.Context(), liveToken); err != nil {
		t.Fatal(err)
	}
	policy, err := upgraded.SessionPolicy(t.Context())
	if err != nil || policy.IdleMinutes != 480 {
		t.Fatal("upgrade did not adopt 8-hour default")
	}
}

func TestIdlePolicyShorteningAndAuthorization(t *testing.T) {
	store, _ := newAccountStore(t)
	now := time.Now().UTC()
	store.now = func() time.Time { return now }
	admin := managementAccount(t, store, "owner", RoleAdmin)
	_, older := managementSession(t, store, admin.ID, SessionClient{})
	now = now.Add(time.Hour)
	_, actor := managementSession(t, store, admin.ID, SessionClient{})
	retired, err := store.SetSessionPolicy(t.Context(), actor.Key, 30)
	if err != nil || len(retired) != 1 || retired[0] != older.Key {
		t.Fatalf("retired=%v err=%v", len(retired), err)
	}
	if _, err := store.SetSessionPolicy(t.Context(), older.Key, 0); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("expired administrator changed policy")
	}
	for _, invalid := range []int{-1, 1, 720, 1440} {
		if _, err := store.SetSessionPolicy(t.Context(), actor.Key, invalid); !errors.Is(err, ErrInvalidSessionPolicy) {
			t.Fatalf("accepted %d", invalid)
		}
	}
	operator := managementAccount(t, store, "observer", RoleOperator)
	_, observer := managementSession(t, store, operator.ID, SessionClient{})
	if _, err := store.SetSessionPolicy(t.Context(), observer.Key, 0); !errors.Is(err, ErrAdministratorRequired) {
		t.Fatal("operator changed installation policy")
	}
}
