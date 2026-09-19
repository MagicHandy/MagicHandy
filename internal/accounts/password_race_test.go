package accounts

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCredentialVerificationRejectsPasswordResetBeforeCommit(t *testing.T) {
	store, _ := newAccountStore(t)
	account, err := store.BootstrapAdmin(t.Context(), "reset-race", "the previous password")
	if err != nil {
		t.Fatal(err)
	}
	verified, release := make(chan struct{}), make(chan struct{})
	var gate sync.Once
	var finish sync.Once
	defer finish.Do(func() { close(release) })
	// Authenticate reads the clock after verifying the password and before
	// entering the writer. Hold exactly that boundary while reset commits.
	store.now = func() time.Time {
		first := false
		gate.Do(func() { first = true })
		if first {
			close(verified)
			<-release
		}
		return time.Now().UTC()
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.Authenticate(t.Context(), account.Username, "the previous password")
		done <- err
	}()
	<-verified
	if err := store.SetPassword(t.Context(), account.ID, "the replacement password"); err != nil {
		t.Fatal(err)
	}
	finish.Do(func() { close(release) })
	if err := <-done; !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a login verified before password reset still succeeded: %v", err)
	}
}

func TestLoginCannotCreateSessionAfterPasswordReset(t *testing.T) {
	store, database := newAccountStore(t)
	account, err := store.BootstrapAdmin(t.Context(), "issue-race", "the previous password")
	if err != nil {
		t.Fatal(err)
	}
	verified, release := make(chan struct{}), make(chan struct{})
	var gate, finish sync.Once
	defer finish.Do(func() { close(release) })
	original := store.randomBytes
	store.randomBytes = func(p []byte) error {
		gate.Do(func() { close(verified); <-release })
		return original(p)
	}
	done := make(chan error, 1)
	go func() {
		token, _, err := store.LoginWithClient(t.Context(), account.Username, "the previous password", SessionClient{})
		if token != "" {
			t.Error("failed login returned a bearer token")
		}
		done <- err
	}()
	<-verified
	if err := store.SetPassword(t.Context(), account.ID, "the replacement password"); err != nil {
		t.Fatal(err)
	}
	finish.Do(func() { close(release) })
	if err := <-done; !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("password reset failed to fence session creation: %v", err)
	}
	var sessions int
	if err := database.SQL().QueryRowContext(t.Context(), `SELECT count(*) FROM user_sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("old proof left a session: count=%d err=%v", sessions, err)
	}
}

func TestLoginAndItsTimestampRollBackTogether(t *testing.T) {
	store, database := newAccountStore(t)
	account, err := store.BootstrapAdmin(t.Context(), "atomic-login", "the login password")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER fail_session_audit BEFORE INSERT ON access_audit
			WHEN json_extract(NEW.document, '$.kind') = 'session_created' BEGIN SELECT RAISE(ABORT, 'injected session audit failure'); END`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if token, _, err := store.LoginWithClient(t.Context(), account.Username, "the login password", SessionClient{}); err == nil || token != "" {
		t.Fatal("a failed session commit succeeded")
	}
	var lastLogin string
	if err := database.SQL().QueryRowContext(t.Context(), `SELECT last_login_at FROM user_accounts WHERE id = ?`, account.ID).Scan(&lastLogin); err != nil || lastLogin != "" {
		t.Fatalf("failed login advanced its timestamp: %q %v", lastLogin, err)
	}
}

func TestLoginPasswordQueueCancellationIsNotAnInvalidPassword(t *testing.T) {
	store, _ := newAccountStore(t)
	store.passwordSlot <- struct{}{}
	defer store.releasePasswordSlot()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := store.LoginWithClient(ctx, "missing", "any password", SessionClient{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was misclassified: %v", err)
	}
}
