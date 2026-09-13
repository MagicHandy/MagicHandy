package accounts

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSessionLimitNeverEvictsNewLoginWhenCreationTimesTie(t *testing.T) {
	store, database := newAccountStore(t)
	owner := managementAccount(t, store, "owner", RoleAdmin)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	// Deliberately issue hashes in descending order so a timestamp/hash-only
	// eviction policy would discard the last login as soon as the cap is hit.
	raw := make([][32]byte, MaxSessionsPerAccount+1)
	for i := range raw {
		raw[i][0] = byte(i + 1)
	}
	sort.Slice(raw, func(i, j int) bool {
		return hashSessionToken(base64.RawURLEncoding.EncodeToString(raw[i][:])) > hashSessionToken(base64.RawURLEncoding.EncodeToString(raw[j][:]))
	})
	index := 0
	store.randomBytes = func(target []byte) error {
		copy(target, raw[index][:])
		if len(target) == 16 {
			index++
		}
		return nil
	}
	for range raw {
		token, _ := managementSession(t, store, owner.ID, SessionClient{})
		if _, err := store.InspectSession(t.Context(), token); err != nil {
			t.Fatalf("new login %d was immediately invalidated: %v", index, err)
		}
	}
	var count int
	if err := database.SQL().QueryRow(`SELECT COUNT(*) FROM user_sessions WHERE user_id = ?`, owner.ID).Scan(&count); err != nil || count != MaxSessionsPerAccount {
		t.Fatalf("session cap: %d, %v", count, err)
	}
}

func managementSession(t *testing.T, store *Store, accountID string, client SessionClient) (string, Session) {
	t.Helper()
	token, session, err := store.NewSessionWithClient(t.Context(), accountID, client)
	if err != nil {
		t.Fatal(err)
	}
	return token, session
}

func managementAccount(t *testing.T, store *Store, name, role string) Account {
	t.Helper()
	account, err := store.Create(t.Context(), name, "synthetic session review passphrase", role)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func TestSessionManagementIDsAreIndependentAndPrivateKeysStayPrivate(t *testing.T) {
	store, _ := newAccountStore(t)
	owner := managementAccount(t, store, "owner", RoleAdmin)
	firstToken, first := managementSession(t, store, owner.ID, SessionClient{Browser: "chrome", Platform: "windows"})
	secondToken, second := managementSession(t, store, owner.ID, SessionClient{Browser: "raw user-agent must not survive", Platform: "private-path"})
	sessions, err := store.ListOwnSessions(t.Context(), first.Key)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("session list: %d, %v", len(sessions), err)
	}
	if !sessions[0].Current || sessions[0].ID != first.ID || sessions[1].Current || first.ID == second.ID || !validSessionManagementID(first.ID) {
		t.Fatal("management identity/current-session mismatch")
	}
	if sessions[1].Client != (SessionClient{Browser: "other", Platform: "other"}) {
		t.Fatal("unbounded client hints were retained")
	}
	data, err := json.Marshal(sessions)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{firstToken, secondToken, first.Key, second.Key, "raw user-agent", "private-path"} {
		if strings.Contains(string(data), private) {
			t.Fatal("private authentication or client material entered session response")
		}
	}
	if _, err := store.ResolveSession(t.Context(), first.ID); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("management ID authenticated as a bearer token")
	}
}

func TestSessionManagementIsAccountScopedEvenForAdministrators(t *testing.T) {
	store, _ := newAccountStore(t)
	admin := managementAccount(t, store, "admin", RoleAdmin)
	operator := managementAccount(t, store, "operator", RoleOperator)
	_, actor := managementSession(t, store, admin.ID, SessionClient{})
	otherToken, other := managementSession(t, store, operator.ID, SessionClient{})
	for _, direction := range [][2]Session{{actor, other}, {other, actor}} {
		if err := store.RenameOwnSession(t.Context(), direction[0].Key, direction[1].ID, "stolen label"); !errors.Is(err, ErrManagedSessionNotFound) {
			t.Fatal("cross-account rename was accepted")
		}
		if _, err := store.RevokeOwnSession(t.Context(), direction[0].Key, direction[1].ID); !errors.Is(err, ErrManagedSessionNotFound) {
			t.Fatal("cross-account revocation was accepted")
		}
	}
	if _, err := store.ResolveSession(t.Context(), otherToken); err != nil {
		t.Fatal("foreign session was changed")
	}
	if err := store.RenameOwnSession(t.Context(), other.Key, other.ID, "  Phone 界  "); err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ListOwnSessions(t.Context(), other.Key)
	if err != nil || len(sessions) != 1 || sessions[0].Name != "Phone 界" {
		t.Fatal("operator cannot manage own session")
	}
	for _, name := range []string{strings.Repeat("界", 81), "line\nbreak", "invalid\xff"} {
		if err := store.RenameOwnSession(t.Context(), other.Key, other.ID, name); !errors.Is(err, ErrInvalidSessionName) {
			t.Fatal("invalid name was accepted")
		}
	}
}

func TestSessionInspectionDoesNotRenewActivityAndOmitsExpiredLogins(t *testing.T) {
	store, database := newAccountStore(t)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	owner := managementAccount(t, store, "owner", RoleAdmin)
	_, older := managementSession(t, store, owner.ID, SessionClient{})
	now = now.Add(20 * time.Minute)
	_, current := managementSession(t, store, owner.ID, SessionClient{})
	now = now.Add(11 * time.Minute)
	sessions, err := store.ListOwnSessions(t.Context(), current.Key)
	if err != nil || len(sessions) != 1 || sessions[0].ID != current.ID {
		t.Fatal("expired login remained in active list")
	}
	var lastSeen string
	if err := database.SQL().QueryRow(`SELECT last_seen_at FROM user_sessions WHERE token_hash = ?`, current.Key).Scan(&lastSeen); err != nil {
		t.Fatal(err)
	}
	if lastSeen != now.Add(-11*time.Minute).Format(time.RFC3339Nano) {
		t.Fatal("passive listing renewed idle time")
	}
	if _, err := store.ListOwnSessions(t.Context(), older.Key); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("expired actor could list sessions")
	}
}

func TestRevokeOtherSessionsPreservesOnlyCallerAndOtherAccounts(t *testing.T) {
	store, _ := newAccountStore(t)
	owner := managementAccount(t, store, "owner", RoleAdmin)
	other := managementAccount(t, store, "other", RoleOperator)
	currentToken, current := managementSession(t, store, owner.ID, SessionClient{})
	peerToken, peer := managementSession(t, store, owner.ID, SessionClient{})
	foreignToken, _ := managementSession(t, store, other.ID, SessionClient{})
	keys, err := store.RevokeOtherSessions(t.Context(), current.Key)
	if err != nil || len(keys) != 1 || keys[0] != peer.Key {
		t.Fatal("unexpected revocation set")
	}
	if _, err := store.ResolveSession(t.Context(), peerToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("peer still authenticated")
	}
	for _, token := range []string{currentToken, foreignToken} {
		if _, err := store.ResolveSession(t.Context(), token); err != nil {
			t.Fatal("unrelated login was revoked")
		}
	}
	if keys, err := store.RevokeOtherSessions(t.Context(), current.Key); err != nil || len(keys) != 0 {
		t.Fatal("repeated bulk revocation is not idempotent")
	}
}

func TestQueuedSessionMutationRechecksActorAfterRevocation(t *testing.T) {
	for _, operation := range []string{"rename", "revoke", "revoke-others"} {
		t.Run(operation, func(t *testing.T) {
			store, database := newAccountStore(t)
			owner := managementAccount(t, store, "owner", RoleAdmin)
			_, actor := managementSession(t, store, owner.ID, SessionClient{})
			peerToken, peer := managementSession(t, store, owner.ID, SessionClient{})
			locked, release, writerDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			go func() {
				writerDone <- database.WithTx(t.Context(), func(tx *sql.Tx) error {
					close(locked)
					<-release
					_, err := tx.Exec(`DELETE FROM user_sessions WHERE token_hash = ?`, actor.Key)
					return err
				})
			}()
			<-locked
			done := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "rename":
					err = store.RenameOwnSession(t.Context(), actor.Key, peer.ID, "obsolete mutation")
				case "revoke":
					_, err = store.RevokeOwnSession(t.Context(), actor.Key, peer.ID)
				default:
					_, err = store.RevokeOtherSessions(t.Context(), actor.Key)
				}
				done <- err
			}()
			close(release)
			if err := <-writerDone; err != nil {
				t.Fatal(err)
			}
			if err := <-done; !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("obsolete mutation result: %v", err)
			}
			if _, err := store.ResolveSession(t.Context(), peerToken); err != nil {
				t.Fatal("peer was revoked after actor lost authority")
			}
			sessions, err := store.ListOwnSessions(t.Context(), peer.Key)
			if err != nil || len(sessions) != 1 || sessions[0].Name != "" {
				t.Fatal("obsolete actor changed peer metadata")
			}
		})
	}
}

func TestSessionManagementCancelsInWriterQueue(t *testing.T) {
	store, database := newAccountStore(t)
	owner := managementAccount(t, store, "owner", RoleAdmin)
	_, actor := managementSession(t, store, owner.ID, SessionClient{})
	_, peer := managementSession(t, store, owner.ID, SessionClient{})
	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- database.WithTx(context.Background(), func(*sql.Tx) error { close(locked); <-release; return nil })
	}()
	<-locked
	t.Cleanup(func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := store.RevokeOwnSession(ctx, actor.Key, peer.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued revocation: %v", err)
	}
}
