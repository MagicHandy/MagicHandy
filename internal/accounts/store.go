// Package accounts owns authenticated user identities and opaque browser
// sessions. It borrows MagicHandy's process-owned SQLite handle; it never owns
// a second database or closes the shared handle.
package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	// RoleAdmin can manage accounts and use the shared application.
	RoleAdmin = "admin"
	// RoleOperator can use the shared application but cannot manage accounts.
	RoleOperator = "operator"

	// DefaultSessionLifetime is the absolute server-side session limit.
	DefaultSessionLifetime = 12 * time.Hour
	// DefaultSessionIdleLimit expires a session not seen by the server.
	DefaultSessionIdleLimit = 30 * time.Minute
	// MaxSessionsPerAccount bounds persistent sessions for one account.
	MaxSessionsPerAccount = 20
)

var (
	// ErrAlreadyInitialized prevents a second first-account bootstrap.
	ErrAlreadyInitialized = errors.New("user accounts are already initialized")
	// ErrInvalidCredentials deliberately covers every failed login reason.
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrInvalidRole marks an unsupported account role.
	ErrInvalidRole = errors.New("invalid account role")
	// ErrInvalidSession deliberately covers missing, expired, and revoked tokens.
	ErrInvalidSession = errors.New("invalid or expired session")
	// ErrInvalidUsername marks a username syntax or length failure.
	ErrInvalidUsername = errors.New("invalid username")
	// ErrLastAdmin prevents loss of the final enabled administrator.
	ErrLastAdmin = errors.New("the last enabled administrator cannot be disabled")
	// ErrNotFound reports a requested account ID that does not exist.
	ErrNotFound = errors.New("user account not found")
	// ErrUsernameTaken reports a case-insensitive username collision.
	ErrUsernameTaken = errors.New("username is already in use")
)

// Account is the non-secret public view of a user identity.
type Account struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	Disabled    bool   `json:"disabled"`
	LastLoginAt string `json:"last_login_at,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Session is an authenticated account plus the server-enforced absolute
// expiration. The raw bearer token is returned only by NewSession.
type Session struct {
	Account   Account   `json:"account"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Store provides account operations over the process-owned datastore.
type Store struct {
	db           *appstore.DB
	now          func() time.Time
	lifetime     time.Duration
	idleLimit    time.Duration
	randomBytes  func([]byte) error
	passwordSlot chan struct{}
}

// New creates the account domain over an existing process-owned datastore.
func New(db *appstore.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("account datastore is required")
	}
	return &Store{
		db:        db,
		now:       func() time.Time { return time.Now().UTC() },
		lifetime:  DefaultSessionLifetime,
		idleLimit: DefaultSessionIdleLimit,
		// Argon2id is intentionally memory-hard. One slot bounds a burst of
		// simultaneous login attempts to one 19 MiB work area.
		passwordSlot: make(chan struct{}, 1),
		randomBytes: func(buffer []byte) error {
			_, err := rand.Read(buffer)
			return err
		},
	}, nil
}

// Initialized reports whether any account exists, including disabled rows.
func (s *Store) Initialized(ctx context.Context) (bool, error) {
	count, err := s.count(ctx, "")
	return count > 0, err
}

// EnabledCount returns the number of accounts allowed to authenticate.
func (s *Store) EnabledCount(ctx context.Context) (int, error) {
	return s.count(ctx, "WHERE disabled = 0")
}

func (s *Store) count(ctx context.Context, condition string) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM user_accounts " + condition // #nosec G202 -- condition is an internal constant.
	if err := s.db.SQL().QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count user accounts: %w", err)
	}
	return count, nil
}

// BootstrapAdmin creates the first account. The emptiness check and insert use
// the shared serialized writer so simultaneous bootstrap requests cannot both
// succeed.
func (s *Store) BootstrapAdmin(ctx context.Context, username, password string) (Account, error) {
	return s.create(ctx, username, password, RoleAdmin, true)
}

// Create adds an account after the HTTP edge has authorized an administrator.
func (s *Store) Create(ctx context.Context, username, password, role string) (Account, error) {
	return s.create(ctx, username, password, role, false)
}

func (s *Store) create(ctx context.Context, username, password, role string, bootstrap bool) (Account, error) {
	username, usernameKey, err := normalizeUsername(username)
	if err != nil {
		return Account{}, err
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role != RoleAdmin && role != RoleOperator {
		return Account{}, fmt.Errorf("%w: role must be admin or operator", ErrInvalidRole)
	}
	passwordHash, err := s.hashPassword(ctx, password)
	if err != nil {
		return Account{}, err
	}
	id, err := s.randomID(16)
	if err != nil {
		return Account{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	account := Account{ID: id, Username: username, Role: role, CreatedAt: now, UpdatedAt: now}
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if bootstrap {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_accounts`).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return ErrAlreadyInitialized
			}
		}
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM user_accounts WHERE username_key = ?`, usernameKey).Scan(&exists)
		if err == nil {
			return ErrUsernameTaken
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO user_accounts(
				id, username, username_key, role, password_hash, disabled,
				last_login_at, created_at, updated_at
			) VALUES(?, ?, ?, ?, ?, 0, '', ?, ?)
		`, id, username, usernameKey, role, passwordHash, now, now)
		return err
	})
	if err != nil {
		return Account{}, fmt.Errorf("create user account: %w", err)
	}
	return account, nil
}

// List returns public account metadata. Password hashes never leave this
// package, even to another backend package.
func (s *Store) List(ctx context.Context) ([]Account, error) {
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT id, username, role, disabled, last_login_at, created_at, updated_at
		FROM user_accounts
		ORDER BY username_key, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list user accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	accounts := make([]Account, 0)
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read user accounts: %w", err)
	}
	return accounts, nil
}

// Authenticate performs the same Argon2id work for missing, disabled, and
// existing accounts and returns one generic credential error.
func (s *Store) Authenticate(ctx context.Context, username, password string) (Account, error) {
	_, usernameKey, usernameErr := normalizeUsername(username)
	var account Account
	var encoded string
	found := usernameErr == nil
	if found {
		var disabled int
		err := s.db.SQL().QueryRowContext(ctx, `
			SELECT id, username, role, disabled, last_login_at, created_at, updated_at, password_hash
			FROM user_accounts
			WHERE username_key = ?
		`, usernameKey).Scan(
			&account.ID, &account.Username, &account.Role, &disabled,
			&account.LastLoginAt, &account.CreatedAt, &account.UpdatedAt, &encoded,
		)
		account.Disabled = disabled != 0
		if errors.Is(err, sql.ErrNoRows) {
			found = false
		} else if err != nil {
			return Account{}, fmt.Errorf("read user account: %w", err)
		}
	}
	if !found {
		encoded = dummyPasswordHash()
	}
	matched, verifyErr := s.verifyPassword(ctx, password, encoded)
	if verifyErr != nil || !found || account.Disabled || !matched {
		return Account{}, ErrInvalidCredentials
	}

	now := s.now().Format(time.RFC3339Nano)
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE user_accounts SET last_login_at = ?, updated_at = ?
			WHERE id = ? AND disabled = 0
		`, now, now, account.ID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return ErrInvalidCredentials
		}
		return nil
	}); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return Account{}, err
		}
		return Account{}, fmt.Errorf("record user login: %w", err)
	}
	account.LastLoginAt = now
	account.UpdatedAt = now
	return account, nil
}

// NewSession creates a high-entropy bearer token while storing only its
// SHA-256 digest. A database disclosure therefore does not reveal live tokens.
func (s *Store) NewSession(ctx context.Context, accountID string) (string, Session, error) {
	raw := make([]byte, 32)
	if err := s.randomBytes(raw); err != nil {
		return "", Session{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	tokenHash := hashSessionToken(token)
	nowTime := s.now()
	now := nowTime.Format(time.RFC3339Nano)
	expires := nowTime.Add(s.lifetime)
	var account Account
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var disabled int
		if err := tx.QueryRowContext(ctx, `
			SELECT id, username, role, disabled, last_login_at, created_at, updated_at
			FROM user_accounts WHERE id = ?
		`, accountID).Scan(
			&account.ID, &account.Username, &account.Role, &disabled,
			&account.LastLoginAt, &account.CreatedAt, &account.UpdatedAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		account.Disabled = disabled != 0
		if account.Disabled {
			return ErrInvalidCredentials
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_sessions(token_hash, user_id, created_at, last_seen_at, expires_at)
			VALUES(?, ?, ?, ?, ?)
		`, tokenHash, accountID, now, now, expires.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			DELETE FROM user_sessions
			WHERE token_hash IN (
				SELECT token_hash FROM user_sessions
				WHERE user_id = ?
				ORDER BY created_at DESC, token_hash DESC
				LIMIT -1 OFFSET ?
			)
		`, accountID, MaxSessionsPerAccount)
		return err
	})
	if err != nil {
		return "", Session{}, fmt.Errorf("create user session: %w", err)
	}
	return token, Session{Account: account, ExpiresAt: expires}, nil
}

// ResolveSession authenticates a token and advances its idle timestamp at most
// once per minute, avoiding a SQLite write for every UI polling request.
func (s *Store) ResolveSession(ctx context.Context, token string) (Session, error) {
	if len(token) < 32 || len(token) > 128 {
		return Session{}, ErrInvalidSession
	}
	var account Account
	var disabled int
	var lastSeenRaw, expiresRaw string
	err := s.db.SQL().QueryRowContext(ctx, `
		SELECT a.id, a.username, a.role, a.disabled, a.last_login_at, a.created_at, a.updated_at,
		       s.last_seen_at, s.expires_at
		FROM user_sessions s
		JOIN user_accounts a ON a.id = s.user_id
		WHERE s.token_hash = ?
	`, hashSessionToken(token)).Scan(
		&account.ID, &account.Username, &account.Role, &disabled,
		&account.LastLoginAt, &account.CreatedAt, &account.UpdatedAt,
		&lastSeenRaw, &expiresRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInvalidSession
	}
	if err != nil {
		return Session{}, fmt.Errorf("read user session: %w", err)
	}
	account.Disabled = disabled != 0
	lastSeen, lastSeenErr := time.Parse(time.RFC3339Nano, lastSeenRaw)
	expires, expiresErr := time.Parse(time.RFC3339Nano, expiresRaw)
	now := s.now()
	if lastSeenErr != nil || expiresErr != nil || account.Disabled || !now.Before(expires) || now.Sub(lastSeen) > s.idleLimit {
		_ = s.RevokeSession(ctx, token)
		return Session{}, ErrInvalidSession
	}
	if now.Sub(lastSeen) >= time.Minute {
		if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
			result, err := tx.ExecContext(ctx, `
				UPDATE user_sessions SET last_seen_at = ?
				WHERE token_hash = ?
			`, now.Format(time.RFC3339Nano), hashSessionToken(token))
			if err != nil {
				return err
			}
			changed, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if changed != 1 {
				return ErrInvalidSession
			}
			return nil
		}); err != nil {
			return Session{}, err
		}
	}
	return Session{Account: account, ExpiresAt: expires}, nil
}

// RevokeSession invalidates one opaque bearer token. It is idempotent.
func (s *Store) RevokeSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE token_hash = ?`, hashSessionToken(token))
		return err
	})
}

// SetPassword replaces a hash and transactionally invalidates every existing
// session for the account.
func (s *Store) SetPassword(ctx context.Context, accountID, password string) error {
	encoded, err := s.hashPassword(ctx, password)
	if err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE user_accounts SET password_hash = ?, updated_at = ? WHERE id = ?
		`, encoded, now, accountID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = ?`, accountID)
		return err
	})
}

// SetDisabled changes login eligibility and revokes sessions when disabling.
// At least one enabled administrator is always retained.
func (s *Store) SetDisabled(ctx context.Context, accountID string, disabled bool) error {
	now := s.now().Format(time.RFC3339Nano)
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var role string
		var currentDisabled int
		if err := tx.QueryRowContext(ctx, `SELECT role, disabled FROM user_accounts WHERE id = ?`, accountID).Scan(&role, &currentDisabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if disabled && currentDisabled == 0 && role == RoleAdmin {
			var admins int
			if err := tx.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM user_accounts WHERE role = 'admin' AND disabled = 0
			`).Scan(&admins); err != nil {
				return err
			}
			if admins <= 1 {
				return ErrLastAdmin
			}
		}
		disabledValue := 0
		if disabled {
			disabledValue = 1
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE user_accounts SET disabled = ?, updated_at = ? WHERE id = ?
		`, disabledValue, now, accountID); err != nil {
			return err
		}
		if disabled {
			_, err := tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = ?`, accountID)
			return err
		}
		return nil
	})
}

func normalizeUsername(username string) (string, string, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 {
		return "", "", fmt.Errorf("%w: username must contain 3 to 64 characters", ErrInvalidUsername)
	}
	for _, character := range username {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return "", "", fmt.Errorf("%w: username may contain only letters, numbers, period, underscore, and hyphen", ErrInvalidUsername)
	}
	return username, strings.ToLower(username), nil
}

func (s *Store) hashPassword(ctx context.Context, password string) (string, error) {
	if err := s.acquirePasswordSlot(ctx); err != nil {
		return "", err
	}
	defer s.releasePasswordSlot()
	return HashPassword(password)
}

func (s *Store) verifyPassword(ctx context.Context, password, encoded string) (bool, error) {
	if err := s.acquirePasswordSlot(ctx); err != nil {
		return false, err
	}
	defer s.releasePasswordSlot()
	matched, err := VerifyPassword(password, encoded)
	if err != nil {
		// A damaged hash must fail closed without turning corruption into a
		// noticeably cheaper username-existence oracle.
		_, _ = VerifyPassword(password, dummyPasswordHash())
	}
	return matched, err
}

func (s *Store) acquirePasswordSlot(ctx context.Context) error {
	select {
	case s.passwordSlot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Store) releasePasswordSlot() {
	<-s.passwordSlot
}

type rowScanner interface {
	Scan(...any) error
}

func scanAccount(row rowScanner) (Account, error) {
	var account Account
	var disabled int
	err := row.Scan(
		&account.ID, &account.Username, &account.Role, &disabled,
		&account.LastLoginAt, &account.CreatedAt, &account.UpdatedAt,
	)
	account.Disabled = disabled != 0
	return account, err
}

func (s *Store) randomID(size int) (string, error) {
	buffer := make([]byte, size)
	if err := s.randomBytes(buffer); err != nil {
		return "", fmt.Errorf("generate account id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func hashSessionToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func dummyPasswordHash() string {
	return "$argon2id$v=19$m=19456,t=2,p=1$TWFnaWNIYW5keUR1bW15IQ$7/PbJeiE3bA/tlZatQ6n079huynNt2ioWji6rxtUY94"
}
