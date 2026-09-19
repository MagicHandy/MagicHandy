package accounts

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

// MaxRecoveryCodes is also enforced by the datastore's insertion trigger.
const MaxRecoveryCodes = 8

// ErrInvalidRecoveryCode intentionally does not distinguish unknown accounts,
// disabled accounts, invalid codes, and codes that have already been consumed.
var ErrInvalidRecoveryCode = errors.New("invalid username or recovery code")

// RecoveryStatus is safe to inspect; it never contains codes or their digests.
type RecoveryStatus struct {
	Remaining int    `json:"remaining"`
	Limit     int    `json:"limit"`
	CreatedAt string `json:"created_at,omitempty"`
}

// RecoveryCodes is a one-time issuance result for the account owner. Existing
// codes cannot be retrieved; diagnostics and exports must use RecoveryStatus.
type RecoveryCodes struct {
	RecoveryStatus
	Codes []string `json:"codes"`
}

// RecoveryStatus returns the signed-in account's saved-code count, never codes.
func (s *Store) RecoveryStatus(ctx context.Context, actorKey string) (RecoveryStatus, error) {
	status := RecoveryStatus{Limit: MaxRecoveryCodes}
	var lastSeen, expires string
	err := s.db.SQL().QueryRowContext(ctx, `SELECT s.last_seen_at, s.expires_at,
		count(c.code_hash), coalesce(max(c.created_at), '') FROM user_sessions s
		JOIN user_accounts a ON a.id = s.user_id AND a.disabled = 0
		LEFT JOIN user_recovery_codes c ON c.user_id = a.id
		WHERE s.token_hash = ? GROUP BY s.token_hash`, actorKey).Scan(&lastSeen, &expires, &status.Remaining, &status.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RecoveryStatus{}, ErrInvalidSession
	}
	if err != nil {
		return RecoveryStatus{}, err
	}
	if _, _, valid := s.sessionTimes(lastSeen, expires); !valid {
		return RecoveryStatus{}, ErrInvalidSession
	}
	if status.Remaining > MaxRecoveryCodes {
		return RecoveryStatus{}, errors.New("account recovery code limit is invalid")
	}
	return status, nil
}

// ReplaceRecoveryCodes requires both a current login and the current password.
// The proof/session are rechecked in the write transaction before replacing all
// older codes. A lost response can be recovered by generating a new set.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, actorKey, password string) (RecoveryCodes, error) {
	proof, err := s.recoveryOwnerProof(ctx, actorKey, password)
	if err != nil {
		return RecoveryCodes{}, err
	}
	codes := make([]string, MaxRecoveryCodes)
	for i := range codes {
		random, err := s.randomID(16)
		if err != nil {
			return RecoveryCodes{}, err
		}
		codes[i] = formatRecoveryCode(random)
	}
	now := s.now()
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.revalidateRecoveryOwner(ctx, tx, actorKey, proof); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, proof.account.ID); err != nil {
			return err
		}
		for _, code := range codes {
			if _, err := tx.ExecContext(ctx, `INSERT INTO user_recovery_codes(code_hash, user_id, created_at) VALUES (?, ?, ?)`,
				recoveryCodeDigest(code), proof.account.ID, now.Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
		return audit.AppendTx(ctx, tx, audit.Event{OccurredAt: now.UnixMilli(), Kind: audit.RecoveryCodesReplaced, Outcome: "success",
			Actor: audit.ActingAccount(ctx, proof.account.ID), TargetAccountID: proof.account.ID, Count: MaxRecoveryCodes})
	})
	if err != nil {
		return RecoveryCodes{}, err
	}
	return RecoveryCodes{RecoveryStatus: RecoveryStatus{Remaining: len(codes), Limit: MaxRecoveryCodes, CreatedAt: now.Format(time.RFC3339Nano)}, Codes: codes}, nil
}

func (s *Store) recoveryOwnerProof(ctx context.Context, actorKey, password string) (passwordProof, error) {
	session, err := s.CheckSession(ctx, actorKey)
	if err != nil {
		return passwordProof{}, err
	}
	return s.checkPassword(ctx, session.Account.Username, password)
}

func (s *Store) revalidateRecoveryOwner(ctx context.Context, tx *sql.Tx, actorKey string, proof passwordProof) error {
	owner, err := s.liveSessionOwner(ctx, tx, actorKey)
	if err != nil {
		return err
	}
	if owner != proof.account.ID {
		return ErrInvalidSession
	}
	return proof.revalidate(ctx, tx)
}

// RemoveRecoveryCodes disables offline recovery after fresh password proof.
func (s *Store) RemoveRecoveryCodes(ctx context.Context, actorKey, password string) error {
	proof, err := s.recoveryOwnerProof(ctx, actorKey, password)
	if err != nil {
		return err
	}
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.revalidateRecoveryOwner(ctx, tx, actorKey, proof); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, proof.account.ID); err != nil {
			return err
		}
		return audit.AppendTx(ctx, tx, audit.Event{Kind: audit.RecoveryCodesRemoved, Outcome: "success", Actor: audit.ActingAccount(ctx, proof.account.ID), TargetAccountID: proof.account.ID})
	})
}

func normalizeRecoveryCode(code string) string {
	if len(code) > 80 {
		return ""
	}
	code = strings.ToLower(strings.NewReplacer("-", "", " ", "", "\r", "", "\n", "", "\t", "").Replace(code))
	decoded, err := hex.DecodeString(code)
	if err != nil || len(decoded) != 16 {
		return ""
	}
	return code
}

func formatRecoveryCode(code string) string {
	parts := make([]string, 0, 8)
	for i := 0; i < len(code); i += 4 {
		parts = append(parts, strings.ToUpper(code[i:i+4]))
	}
	return strings.Join(parts, "-")
}

func recoveryCodeDigest(code string) string {
	digest := sha256.Sum256([]byte("magichandy-recovery-v1\x00" + normalizeRecoveryCode(code)))
	return hex.EncodeToString(digest[:])
}
