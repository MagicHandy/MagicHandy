package accounts

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestAdministratorWritesRequireCurrentSession(t *testing.T) {
	for _, state := range []string{"revoked", "disabled", "expired", "operator", "missing"} {
		t.Run(state, func(t *testing.T) {
			store, admin, peer, target := administratorFixture(t)
			ctx := t.Context()
			original, err := store.GrantPermanentControl(ctx, peer.ID, target.ID)
			if err != nil {
				t.Fatal(err)
			}
			actor := admin
			if state == "operator" {
				actor = target
			}
			token, session, err := store.NewSession(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := ErrInvalidSession
			switch state {
			case "revoked":
				err = store.RevokeSession(ctx, token)
			case "disabled":
				err = store.SetDisabled(ctx, actor.ID, true)
			case "expired":
				store.now = func() time.Time { return session.ExpiresAt.Add(time.Second) }
			case "operator":
				want = ErrAdministratorRequired
			case "missing":
				session.Key = ""
			}
			if err != nil {
				t.Fatal(err)
			}
			// Use a live, uncancelled context: cancellation alone must never be
			// the authority check for a queued durable write.
			mutations := map[string]func() error{
				"create": func() error {
					_, err := store.CreateForSession(ctx, session.Key, "unauthorized-owner", recoveryTestPassword, RoleAdmin)
					return err
				},
				"enable": func() error {
					_, err := store.SetDisabledForSession(ctx, session.Key, admin.ID, false)
					return err
				},
				"grant": func() error {
					_, err := store.GrantControlForSession(ctx, session.Key, target.ID, time.Hour)
					return err
				},
				"revoke": func() error { return store.RevokeControlForSession(ctx, session.Key, target.ID) },
				"configuration": func() error {
					return store.WithConfirmedAdministrator(ctx, session.Key, recoveryTestPassword, func(*sql.Tx) error {
						t.Error("unauthorized configuration callback executed")
						return nil
					})
				},
			}
			for name, mutate := range mutations {
				if err := mutate(); !errors.Is(err, want) {
					t.Errorf("%s error = %v, want %v", name, err, want)
				}
			}
			assertAdministratorUnchanged(t, store, admin.ID, state == "disabled")
			grant, err := store.ControlGrant(ctx, target.ID)
			if err != nil || grant == nil || grant.ID != original.ID {
				t.Fatal("permission changed", err)
			}
		})
	}
}

func administratorFixture(t *testing.T) (*Store, Account, Account, Account) {
	t.Helper()
	store, _ := newAccountStore(t)
	admin, err := store.BootstrapAdmin(t.Context(), "owner", recoveryTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := store.Create(t.Context(), "other-owner", recoveryTestPassword, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.Create(t.Context(), "observer", recoveryTestPassword, RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	return store, admin, peer, target
}

func assertAdministratorUnchanged(t *testing.T, store *Store, adminID string, disabled bool) {
	t.Helper()
	listed, err := store.List(t.Context())
	if err != nil || len(listed) != 3 {
		t.Fatalf("account creation persisted: %d, %v", len(listed), err)
	}
	for _, account := range listed {
		if account.ID == adminID && account.Disabled != disabled {
			t.Fatal("enabled state changed")
		}
	}
}

func TestAdministratorCreateRechecksAfterPasswordQueue(t *testing.T) {
	store, _, _, token, _ := recoveryFixture(t)
	store.passwordSlot <- struct{}{}
	var release sync.Once
	defer release.Do(store.releasePasswordSlot)
	done := make(chan error, 1)
	go func() {
		_, err := store.CreateForSession(t.Context(), hashSessionToken(token), "queued-owner", recoveryTestPassword, RoleAdmin)
		done <- err
	}()
	if err := store.RevokeSession(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	release.Do(store.releasePasswordSlot)
	if err := <-done; !errors.Is(err, ErrInvalidSession) {
		t.Fatal("queued creation survived sign-out", err)
	}
}

func TestConfirmedAdministratorRechecksAfterPasswordQueue(t *testing.T) {
	store, _, _, token, _ := recoveryFixture(t)
	checked := make(chan struct{})
	var once sync.Once
	store.now = func() time.Time {
		once.Do(func() { close(checked) })
		return time.Now().UTC()
	}
	store.passwordSlot <- struct{}{}
	var release sync.Once
	defer release.Do(store.releasePasswordSlot)
	done := make(chan error, 1)
	go func() {
		done <- store.WithConfirmedAdministrator(t.Context(), hashSessionToken(token), recoveryTestPassword, func(*sql.Tx) error {
			t.Error("revoked confirmed write ran")
			return nil
		})
	}()
	<-checked
	if err := store.RevokeSession(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	release.Do(store.releasePasswordSlot)
	if err := <-done; !errors.Is(err, ErrInvalidSession) {
		t.Fatal("queued confirmation survived sign-out", err)
	}
}

func TestAdministratorDisableReturnsRetiredSessionsAndRetainsLastAdmin(t *testing.T) {
	store, _, admin, token, _ := recoveryFixture(t)
	ctx := context.Background()
	key := hashSessionToken(token)
	if _, err := store.SetDisabledForSession(ctx, key, admin.ID, true); !errors.Is(err, ErrLastAdmin) {
		t.Fatal("last administrator was not protected", err)
	}
	peer, err := store.CreateForSession(ctx, key, "other-owner", recoveryTestPassword, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	_, first, err := store.NewSession(ctx, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := store.NewSession(ctx, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := store.SetDisabledForSession(ctx, first.Key, peer.ID, true)
	if err != nil || len(keys) != 2 || !slices.Contains(keys, first.Key) || !slices.Contains(keys, second.Key) {
		t.Fatal("self-disable did not return all retired sessions", err)
	}
	for _, retired := range keys {
		if _, err := store.CheckSession(ctx, retired); !errors.Is(err, ErrInvalidSession) {
			t.Fatal("disabled session survived", err)
		}
	}
	keys, err = store.SetDisabledForSession(ctx, key, peer.ID, false)
	if err != nil || len(keys) != 0 {
		t.Fatal("live peer could not re-enable account", err)
	}
	if _, err := store.CheckSession(ctx, key); err != nil {
		t.Fatal("peer administration retired unrelated session", err)
	}
}
