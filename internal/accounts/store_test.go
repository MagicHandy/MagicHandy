package accounts

import (
	"context"
	"errors"
	"testing"
	"time"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
)

func newAccountStore(t *testing.T) (*Store, *appstore.DB) {
	t.Helper()
	database, err := appstore.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open datastore: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	accounts, err := New(database)
	if err != nil {
		t.Fatalf("New accounts: %v", err)
	}
	return accounts, database
}

func TestBootstrapAdminIsAtomicAndCredentialsAuthenticate(t *testing.T) {
	accounts, database := newAccountStore(t)
	ctx := context.Background()
	admin, err := accounts.BootstrapAdmin(ctx, "Owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if admin.Role != RoleAdmin || admin.Disabled || admin.ID == "" {
		t.Fatalf("admin = %+v", admin)
	}
	if _, err := accounts.BootstrapAdmin(ctx, "Second", "another excellent password"); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second bootstrap error = %v, want already initialized", err)
	}

	authenticated, err := accounts.Authenticate(ctx, "owner", "correct horse battery staple")
	if err != nil || authenticated.ID != admin.ID || authenticated.LastLoginAt == "" {
		t.Fatalf("Authenticate = (%+v, %v)", authenticated, err)
	}
	for _, test := range []struct{ username, password string }{
		{"owner", "incorrect password"},
		{"missing", "correct horse battery staple"},
		{"not valid!", "correct horse battery staple"},
	} {
		if _, err := accounts.Authenticate(ctx, test.username, test.password); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Authenticate(%q) error = %v, want generic credentials error", test.username, err)
		}
	}
	var storedHash string
	if err := database.SQL().QueryRow(`SELECT password_hash FROM user_accounts WHERE id = ?`, admin.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read password hash: %v", err)
	}
	if storedHash == "correct horse battery staple" || storedHash[:10] != "$argon2id$" {
		t.Fatalf("stored password is not an Argon2id hash: %q", storedHash)
	}
}

func TestAccountNamesAreCaseInsensitiveAndPublicListExcludesHashes(t *testing.T) {
	accounts, database := newAccountStore(t)
	ctx := context.Background()
	if _, err := accounts.BootstrapAdmin(ctx, "Owner", "correct horse battery staple"); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if _, err := accounts.Create(ctx, "OWNER", "another excellent password", RoleOperator); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("case duplicate error = %v, want username taken", err)
	}
	operator, err := accounts.Create(ctx, "operator-1", "another excellent password", RoleOperator)
	if err != nil {
		t.Fatalf("Create operator: %v", err)
	}
	listed, err := accounts.List(ctx)
	if err != nil || len(listed) != 2 || listed[0].Username != "operator-1" || listed[1].Username != "Owner" {
		t.Fatalf("List = (%+v, %v)", listed, err)
	}
	var hash string
	if err := database.SQL().QueryRow(`SELECT password_hash FROM user_accounts WHERE id = ?`, operator.ID).Scan(&hash); err != nil || hash == "" {
		t.Fatalf("stored operator hash = (%q, %v)", hash, err)
	}
}

func TestSessionsStoreOnlyDigestsExpireAndRevokeOnPasswordChange(t *testing.T) {
	accounts, database := newAccountStore(t)
	ctx := context.Background()
	clock := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	accounts.now = func() time.Time { return clock }
	admin, err := accounts.BootstrapAdmin(ctx, "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	token, session, err := accounts.NewSession(ctx, admin.ID)
	if err != nil || token == "" || session.Account.ID != admin.ID {
		t.Fatalf("NewSession = (%q, %+v, %v)", token, session, err)
	}
	var stored string
	if err := database.SQL().QueryRow(`SELECT token_hash FROM user_sessions`).Scan(&stored); err != nil {
		t.Fatalf("read session digest: %v", err)
	}
	if stored == token || stored != hashSessionToken(token) {
		t.Fatalf("stored token = %q, raw = %q", stored, token)
	}
	resolved, err := accounts.ResolveSession(ctx, token)
	if err != nil || resolved.Account.ID != admin.ID {
		t.Fatalf("ResolveSession = (%+v, %v)", resolved, err)
	}

	if err := accounts.SetPassword(ctx, admin.ID, "replacement secure passphrase"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if _, err := accounts.ResolveSession(ctx, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("old session error = %v, want invalid", err)
	}
	if _, err := accounts.Authenticate(ctx, "owner", "correct horse battery staple"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password error = %v, want invalid", err)
	}

	token, _, err = func() (string, Session, error) {
		account, authErr := accounts.Authenticate(ctx, "owner", "replacement secure passphrase")
		if authErr != nil {
			return "", Session{}, authErr
		}
		return accounts.NewSession(ctx, account.ID)
	}()
	if err != nil {
		t.Fatalf("replacement session: %v", err)
	}
	clock = clock.Add(DefaultSessionIdleLimit + time.Second)
	if _, err := accounts.ResolveSession(ctx, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("idle session error = %v, want invalid", err)
	}
}

func TestDisablingAccountsRevokesSessionsAndKeepsAnAdmin(t *testing.T) {
	accounts, _ := newAccountStore(t)
	ctx := context.Background()
	first, err := accounts.BootstrapAdmin(ctx, "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if err := accounts.SetDisabled(ctx, first.ID, true); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disable last admin error = %v, want last admin", err)
	}
	second, err := accounts.Create(ctx, "backup", "another excellent password", RoleAdmin)
	if err != nil {
		t.Fatalf("Create backup admin: %v", err)
	}
	token, _, err := accounts.NewSession(ctx, first.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := accounts.SetDisabled(ctx, first.ID, true); err != nil {
		t.Fatalf("disable admin: %v", err)
	}
	if _, err := accounts.ResolveSession(ctx, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("disabled session error = %v, want invalid", err)
	}
	if _, err := accounts.Authenticate(ctx, "owner", "correct horse battery staple"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled login error = %v, want generic credentials error", err)
	}
	if err := accounts.SetDisabled(ctx, second.ID, true); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disable remaining admin error = %v, want last admin", err)
	}
}

func TestPasswordWorkAdmissionIsBoundedAndCancelable(t *testing.T) {
	accounts, _ := newAccountStore(t)
	accounts.passwordSlot <- struct{}{}
	defer accounts.releasePasswordSlot()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := accounts.hashPassword(ctx, "correct horse battery staple"); !errors.Is(err, context.Canceled) {
		t.Fatalf("queued password hash error = %v, want context cancellation", err)
	}
}

func TestSessionAbsoluteLifetimeAndPerAccountCap(t *testing.T) {
	accounts, database := newAccountStore(t)
	ctx := context.Background()
	clock := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	accounts.now = func() time.Time { return clock }
	accounts.lifetime = time.Hour
	accounts.idleLimit = 2 * time.Hour
	admin, err := accounts.BootstrapAdmin(ctx, "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	token, _, err := accounts.NewSession(ctx, admin.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	clock = clock.Add(time.Hour + time.Second)
	if _, err := accounts.ResolveSession(ctx, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("absolute-expiry error = %v, want invalid session", err)
	}

	clock = clock.Add(time.Hour)
	for index := 0; index < MaxSessionsPerAccount+3; index++ {
		clock = clock.Add(time.Second)
		if _, _, err := accounts.NewSession(ctx, admin.ID); err != nil {
			t.Fatalf("NewSession %d: %v", index, err)
		}
	}
	var count int
	if err := database.SQL().QueryRow(`SELECT COUNT(*) FROM user_sessions WHERE user_id = ?`, admin.ID).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != MaxSessionsPerAccount {
		t.Fatalf("session count = %d, want %d", count, MaxSessionsPerAccount)
	}
}
