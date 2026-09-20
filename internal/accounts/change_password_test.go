package accounts

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdmittedPasswordChangeCannotUndoAccountRecovery(t *testing.T) {
	store, _, account, token, issued := recoveryFixture(t)
	verified, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var finish sync.Once
	defer finish.Do(func() { close(release) })
	// The first clock read validates the session. The second follows both
	// password hashes and precedes the write transaction; recovery wins there.
	store.now = func() time.Time {
		if calls.Add(1) == 2 {
			close(verified)
			<-release
		}
		return time.Now().UTC()
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.ChangeOwnPassword(t.Context(), hashSessionToken(token), recoveryTestPassword, "stale requested password")
		done <- err
	}()
	<-verified
	if _, err := store.RecoverPassword(t.Context(), account.Username, issued.Codes[0], "recovered account password"); err != nil {
		t.Fatal(err)
	}
	finish.Do(func() { close(release) })
	if err := <-done; !errors.Is(err, ErrInvalidSession) {
		t.Fatal("old password change survived recovery", err)
	}
	if _, err := store.Authenticate(t.Context(), account.Username, "recovered account password"); err != nil {
		t.Fatal("recovery was overwritten", err)
	}
}

func TestAdministratorResetRevalidatesItsOwnSession(t *testing.T) {
	store, _, _, token, _ := recoveryFixture(t)
	target, err := store.Create(t.Context(), "reset-target", "original target password", RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	// Hold the expensive hashing slot while the administrator signs out. A
	// later admitted write must recheck authority after leaving this queue.
	store.passwordSlot <- struct{}{}
	var released sync.Once
	defer released.Do(store.releasePasswordSlot)
	done := make(chan error, 1)
	go func() {
		_, err := store.ResetPasswordForSession(t.Context(), hashSessionToken(token), target.ID, "stale administrator password")
		done <- err
	}()
	if err := store.RevokeSession(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	released.Do(store.releasePasswordSlot)
	if err := <-done; !errors.Is(err, ErrInvalidSession) {
		t.Fatal("revoked administrator changed credentials", err)
	}
	if _, err := store.Authenticate(t.Context(), target.Username, "original target password"); err != nil {
		t.Fatal("target credentials changed", err)
	}
}

func TestPasswordMutationRequiresRoleAndReturnsAllRetiredSessions(t *testing.T) {
	store, _, _, adminToken, _ := recoveryFixture(t)
	operator, err := store.Create(t.Context(), "password-operator", "original operator password", RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	_, login, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResetPasswordForSession(t.Context(), login.Key, operator.ID, "unauthorized reset password"); err == nil {
		t.Fatal("operator used administrator reset")
	}
	if _, err := store.ChangeOwnPassword(t.Context(), login.Key, "incorrect current password", "unauthorized self password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("current password was not checked", err)
	}
	keys, err := store.ResetPasswordForSession(t.Context(), hashSessionToken(adminToken), operator.ID, "reset by administrator")
	if err != nil || len(keys) != 1 || keys[0] != login.Key {
		t.Fatal("reset did not return exact retired login", err)
	}
	if _, err := store.CheckSession(t.Context(), hashSessionToken(adminToken)); err != nil {
		t.Fatal("peer reset ended the administrator", err)
	}
	_, login, err = store.LoginWithClient(t.Context(), operator.Username, "reset by administrator", SessionClient{})
	if err != nil {
		t.Fatal(err)
	}
	keys, err = store.ChangeOwnPassword(t.Context(), login.Key, "reset by administrator", "new operator password")
	if err != nil || len(keys) != 1 || keys[0] != login.Key {
		t.Fatal("self password change did not retire login", err)
	}
}
