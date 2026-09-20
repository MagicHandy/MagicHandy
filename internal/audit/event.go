// Package audit owns bounded access/control history. Events have reviewed
// fields and codes; credentials, content, request URLs and raw failures have no
// representation here. It borrows the process datastore and cannot drive motion.
package audit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Kind identifies a reviewed access or control transition.
type Kind string

// Supported event kinds deliberately exclude free-form application content.
const (
	AccountCreated           Kind = "account_created"
	AccountEnabled           Kind = "account_enabled"
	AccountDisabled          Kind = "account_disabled"
	PasswordChanged          Kind = "password_changed"
	PasswordRecovered        Kind = "password_recovered"
	RecoveryCodesReplaced    Kind = "recovery_codes_replaced"
	RecoveryCodesRemoved     Kind = "recovery_codes_removed"
	SessionCreated           Kind = "session_created"
	SessionRevoked           Kind = "session_revoked"
	SessionsRevoked          Kind = "sessions_revoked"
	GrantIssued              Kind = "grant_issued"
	GrantRevoked             Kind = "grant_revoked"
	LoginFailed              Kind = "login_failed"
	LoginThrottled           Kind = "login_throttled"
	CredentialCheckFailed    Kind = "credential_check_failed"    // #nosec G101 -- public event classification, not a credential.
	CredentialCheckThrottled Kind = "credential_check_throttled" // #nosec G101 -- public event classification, not a credential.
	ControlClaimed           Kind = "control_claimed"
	ControlTransferred       Kind = "control_transferred"
	ControlLost              Kind = "control_lost"
	CommandFinished          Kind = "command_finished"
	StopFinished             Kind = "stop_finished"
	ServerStarted            Kind = "server_started"
	ServerStopped            Kind = "server_stopped"
	HistoryGap               Kind = "history_gap"
)

// Actor identifies the authenticated caller or a non-account system/public lane.
type Actor struct {
	Type      string `json:"type"`
	AccountID string `json:"account_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// Event contains only reviewed fields. IDs are server-issued non-authenticating
// references. Correlation is a
// digest of the command ID, never the command body, token or private session key.
type Event struct {
	ID              string `json:"id"`
	Sequence        int64  `json:"sequence"`
	OccurredAt      int64  `json:"occurred_at_ms"`
	Kind            Kind   `json:"kind"`
	Outcome         string `json:"outcome"`
	Actor           Actor  `json:"actor"`
	TargetAccountID string `json:"target_account_id,omitempty"`
	TargetSessionID string `json:"target_session_id,omitempty"`
	GrantID         string `json:"grant_id,omitempty"`
	Operation       string `json:"operation,omitempty"`
	Epoch           string `json:"epoch,omitempty"`
	Generation      uint64 `json:"generation,omitempty"`
	StopSequence    uint64 `json:"stop_sequence,omitempty"`
	TraceSequence   uint64 `json:"trace_sequence,omitempty"`
	Correlation     string `json:"correlation,omitempty"`
	HTTPStatus      int    `json:"http_status,omitempty"`
	Count           uint64 `json:"count,omitempty"`
	ExpiresAt       int64  `json:"expires_at_ms,omitempty"`
	Permanent       bool   `json:"permanent,omitempty"`
}

type actorKey struct{}

// WithActor carries server-resolved attribution into an owning transaction.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// ContextActor returns the caller attribution, defaulting to system work.
func ContextActor(ctx context.Context) Actor {
	if actor, ok := ctx.Value(actorKey{}).(Actor); ok {
		return actor
	}
	return Actor{Type: "system"}
}

// ActingAccount records the account checked by a domain write, retaining a
// session reference only when the context belongs to that same account.
func ActingAccount(ctx context.Context, id string) Actor {
	actor := ContextActor(ctx)
	if actor.AccountID != id {
		actor.SessionID = ""
	}
	actor.Type, actor.AccountID = "account", id
	return actor
}

// CorrelateCommand produces a domain-separated digest without retaining raw IDs.
func CorrelateCommand(id string) string {
	if id == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("audit-command-v1\x00" + id))
	return hex.EncodeToString(digest[:])
}

func prepare(event Event) (Event, []byte, error) {
	if event.ID == "" {
		event.ID = rand.Text()
	}
	if event.OccurredAt == 0 {
		event.OccurredAt = time.Now().UnixMilli()
	}
	if event.Actor.Type == "" {
		event.Actor.Type = "system"
	}
	if !validKind(event.Kind) || !oneOf(event.Outcome, "success", "rejected", "failed", "unconfirmed", "unknown", "incomplete") || !validActor(event.Actor) {
		return Event{}, nil, errors.New("invalid audit event classification")
	}
	if !validReferences(event) {
		return Event{}, nil, errors.New("invalid audit event reference")
	}
	if !oneOf(event.Operation, "", "motion", "mode", "chat", "media", "feedback", "preferences", "host", "heartbeat_expired", "session_ended", "permission_ended", "takeover", "emergency", "shutdown", "other") {
		return Event{}, nil, errors.New("invalid audit operation")
	}
	if event.OccurredAt < 0 || event.OccurredAt > 253402300799999 || event.ExpiresAt < 0 || event.ExpiresAt > 253402300799999 || event.Sequence < 0 || event.HTTPStatus < 0 || event.HTTPStatus > 599 {
		return Event{}, nil, errors.New("invalid audit event numeric value")
	}
	event.Sequence = 0
	encoded, err := json.Marshal(event)
	if err != nil || len(encoded) > 2048 {
		return Event{}, nil, errors.New("audit event exceeds its encoded limit")
	}
	return event, encoded, nil
}

func validReferences(event Event) bool {
	return reference(event.ID, 26) && hexID(event.TargetAccountID, 32) && reference(event.TargetSessionID, 22) && hexID(event.GrantID, 32) && reference(event.Epoch, 26) && hexID(event.Correlation, 64)
}

func validActor(actor Actor) bool {
	if actor.Type == "account" {
		return len(actor.AccountID) == 32 && hexID(actor.AccountID, 32) && reference(actor.SessionID, 22)
	}
	return oneOf(actor.Type, "system", "local", "public") && actor.AccountID == "" && actor.SessionID == ""
}

func validKind(kind Kind) bool {
	switch kind {
	case AccountCreated, AccountEnabled, AccountDisabled, PasswordChanged, PasswordRecovered, RecoveryCodesReplaced, RecoveryCodesRemoved, SessionCreated, SessionRevoked, SessionsRevoked, GrantIssued, GrantRevoked, LoginFailed, LoginThrottled, CredentialCheckFailed, CredentialCheckThrottled, ControlClaimed, ControlTransferred, ControlLost, CommandFinished, StopFinished, ServerStarted, ServerStopped, HistoryGap:
		return true
	default:
		return false
	}
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func reference(value string, length int) bool {
	return value == "" || len(value) == length && strings.IndexFunc(value, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_'
	}) < 0
}

func hexID(value string, length int) bool {
	if value == "" {
		return true
	}
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
