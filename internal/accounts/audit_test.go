package accounts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

func TestAccessChangesCommitAttributionWithoutCredentials(t *testing.T) {
	s, db := newAccountStore(t)
	admin, err := s.BootstrapAdmin(t.Context(), "owner", "private audit test passphrase")
	if err != nil {
		t.Fatal(err)
	}
	token, session, err := s.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := audit.WithActor(t.Context(), audit.Actor{Type: "account", AccountID: admin.ID, SessionID: session.ID})
	operator, err := s.Create(ctx, "operator", "another private passphrase", RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.GrantControl(ctx, admin.ID, operator.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeControl(ctx, admin.ID, operator.ID); err != nil {
		t.Fatal(err)
	}
	page, err := audit.NewStore(db).Page(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[audit.Kind]bool{}
	for _, event := range page.Events {
		if event.Kind != audit.GrantIssued && event.Kind != audit.GrantRevoked {
			continue
		}
		if event.Actor.AccountID != admin.ID || event.Actor.SessionID != session.ID || event.TargetAccountID != operator.ID || event.GrantID != grant.ID {
			t.Fatal("grant lost its actor or target attribution")
		}
		found[event.Kind] = true
	}
	if !found[audit.GrantIssued] || !found[audit.GrantRevoked] {
		t.Fatal("successful permission transitions were omitted")
	}
	data, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{token, session.Key, "private audit test passphrase", "another private passphrase", "password_hash"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("audit serialized a credential")
		}
	}
}

func TestAccessMutationRollsBackWhenItsAuditRecordCannotPersist(t *testing.T) {
	s, db := newAccountStore(t)
	admin, err := s.BootstrapAdmin(t.Context(), "owner", "original audit passphrase")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := s.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`CREATE TRIGGER reject_audit_fixture BEFORE INSERT ON access_audit BEGIN SELECT RAISE(ABORT, 'audit fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword(t.Context(), admin.ID, "replacement audit passphrase"); err == nil {
		t.Fatal("password changed without its required audit event")
	}
	if _, err := s.InspectSession(context.Background(), token); err != nil {
		t.Fatal("rolled-back password change revoked the original session", err)
	}
	if _, err := s.Authenticate(t.Context(), "owner", "original audit passphrase"); err != nil {
		t.Fatal("rolled-back password hash was not preserved", err)
	}
}
