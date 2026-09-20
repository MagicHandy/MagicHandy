package accounts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const recoveryTestPassword = "synthetic recovery fixture password"

func recoveryFixture(t *testing.T) (*Store, *appstore.DB, Account, string, RecoveryCodes) {
	t.Helper()
	store, database := newAccountStore(t)
	account, err := store.BootstrapAdmin(t.Context(), "recovery-owner", recoveryTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	token, session, err := store.LoginWithClient(t.Context(), account.Username, recoveryTestPassword, SessionClient{})
	if err != nil {
		t.Fatal(err)
	}
	codes, err := store.ReplaceRecoveryCodes(t.Context(), session.Key, recoveryTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	return store, database, account, token, codes
}

func TestRecoveryCodesAreBoundedPrivateAndReplaceable(t *testing.T) {
	store, database, _, token, issued := recoveryFixture(t)
	if len(issued.Codes) != MaxRecoveryCodes {
		t.Fatal("wrong recovery code count")
	}
	status, err := store.RecoveryStatus(t.Context(), hashSessionToken(token))
	if err != nil || status.Remaining != MaxRecoveryCodes {
		t.Fatal("recovery status unavailable", err)
	}
	public, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, code := range issued.Codes {
		if seen[code] || len(normalizeRecoveryCode(code)) != 32 || strings.Contains(string(public), code) {
			t.Fatal("recovery code repeated, malformed or disclosed by status")
		}
		seen[code] = true
		var stored string
		if err := database.SQL().QueryRowContext(t.Context(), `SELECT code_hash FROM user_recovery_codes WHERE code_hash = ?`, recoveryCodeDigest(code)).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored == normalizeRecoveryCode(code) || stored == code {
			t.Fatal("recovery code was stored in plaintext")
		}
	}
	if _, err := store.ReplaceRecoveryCodes(t.Context(), hashSessionToken(token), "incorrect password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("existing login alone replaced recovery credentials")
	}
	if _, err := store.ReplaceRecoveryCodes(t.Context(), hashSessionToken(token), recoveryTestPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverPassword(t.Context(), "recovery-owner", issued.Codes[0], "replacement password"); !errors.Is(err, ErrInvalidRecoveryCode) {
		t.Fatal("a replaced recovery code remained usable")
	}
}

func TestRecoveryResetsPasswordAndAllCredentialsAtomically(t *testing.T) {
	store, database, account, token, issued := recoveryFixture(t)
	otherToken, _, err := store.LoginWithClient(t.Context(), account.Username, recoveryTestPassword, SessionClient{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverPassword(t.Context(), "someone-else", issued.Codes[0], "replacement password"); !errors.Is(err, ErrInvalidRecoveryCode) {
		t.Fatal("recovery code was not account-bound")
	}
	result, err := store.RecoverPassword(t.Context(), account.Username, strings.ToLower(issued.Codes[0]), "replacement password")
	if err != nil || result.AccountID != account.ID || len(result.SessionKeys) != 2 {
		t.Fatal("account recovery failed", err)
	}
	for _, revoked := range []string{token, otherToken} {
		if _, err := store.ResolveSession(t.Context(), revoked); !errors.Is(err, ErrInvalidSession) {
			t.Fatal("recovery retained an old login")
		}
	}
	var sessions, codes int
	if err := database.SQL().QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM user_sessions), (SELECT count(*) FROM user_recovery_codes)`).Scan(&sessions, &codes); err != nil || sessions != 0 || codes != 0 {
		t.Fatal("recovery retained credentials or created an automatic login", err)
	}
	if _, err := store.Authenticate(t.Context(), account.Username, recoveryTestPassword); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("old password remained usable")
	}
	if _, _, err := store.LoginWithClient(t.Context(), account.Username, "replacement password", SessionClient{}); err != nil {
		t.Fatal("replacement password did not log in", err)
	}
}

func TestRecoveryRedeemsOnlyOnceUnderConcurrency(t *testing.T) {
	store, _, account, _, issued := recoveryFixture(t)
	results := make(chan error, MaxRecoveryCodes)
	var callers sync.WaitGroup
	for _, code := range issued.Codes {
		callers.Go(func() {
			_, err := store.RecoverPassword(t.Context(), account.Username, code, "replacement password")
			results <- err
		})
	}
	callers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrInvalidRecoveryCode) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent recovery reused a spent credential set")
	}
}

func TestRecoveryAuditFailureRollsBackPasswordAndCodes(t *testing.T) {
	store, database, account, token, issued := recoveryFixture(t)
	if err := database.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER fail_recovery_audit BEFORE INSERT ON access_audit
			WHEN json_extract(NEW.document, '$.kind') = 'password_recovered' BEGIN SELECT RAISE(ABORT, 'injected recovery audit failure'); END`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverPassword(t.Context(), account.Username, issued.Codes[0], "replacement password"); err == nil {
		t.Fatal("recovery succeeded without its durable audit record")
	}
	if _, err := store.ResolveSession(t.Context(), token); err != nil {
		t.Fatal("failed recovery revoked a session", err)
	}
	status, err := store.RecoveryStatus(t.Context(), hashSessionToken(token))
	if err != nil || status.Remaining != MaxRecoveryCodes {
		t.Fatal("failed recovery consumed codes", err)
	}
	if _, err := store.Authenticate(t.Context(), account.Username, recoveryTestPassword); err != nil {
		t.Fatal("failed recovery changed the password", err)
	}
}

func TestQueuedRecoveryReplacementRechecksTheLogin(t *testing.T) {
	store, _, _, token, _ := recoveryFixture(t)
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
		_, err := store.ReplaceRecoveryCodes(t.Context(), hashSessionToken(token), recoveryTestPassword)
		done <- err
	}()
	<-verified
	if err := store.RevokeSession(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	finish.Do(func() { close(release) })
	if err := <-done; !errors.Is(err, ErrInvalidSession) {
		t.Fatal("an ended session minted recovery credentials", err)
	}
}

func TestPasswordChangeInvalidatesSavedRecoveryCodes(t *testing.T) {
	store, _, account, _, issued := recoveryFixture(t)
	if err := store.SetPassword(t.Context(), account.ID, "replacement password"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverPassword(t.Context(), account.Username, issued.Codes[0], "another password"); !errors.Is(err, ErrInvalidRecoveryCode) {
		t.Fatal("password change retained an older recovery credential")
	}
}
