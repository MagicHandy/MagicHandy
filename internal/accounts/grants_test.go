package accounts

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
)

func newGrantFixture(t *testing.T) (*Store, *appstore.DB, Account, Account) {
	t.Helper()
	store, db := newAccountStore(t)
	admin, err := store.BootstrapAdmin(t.Context(), "owner", "synthetic owner passphrase")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := store.Create(t.Context(), "observer", "synthetic observer passphrase", RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	return store, db, admin, operator
}

func TestPermanentControlPersistsWithoutExtendingLogin(t *testing.T) {
	store, db, admin, operator := newGrantFixture(t)
	grant, err := store.GrantPermanentControl(t.Context(), admin.ID, operator.ID)
	if err != nil || grant.ExpiresAt != nil {
		t.Fatalf("permanent permission: %+v %v", grant, err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A new account store and a far-future clock must retain the durable grant,
	// while the existing login expires under its ordinary absolute/idle limits.
	reopened, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().AddDate(10, 0, 0)
	reopened.now = func() time.Time { return future }
	persisted, err := reopened.ControlGrant(t.Context(), operator.ID)
	if err != nil || persisted == nil || persisted.ID != grant.ID || persisted.ExpiresAt != nil {
		t.Fatalf("durable permission: %+v %v", persisted, err)
	}
	if !(Session{Account: operator, ControlGrant: persisted}).CanControl(future) {
		t.Fatal("permanent grant expired")
	}
	if _, err = reopened.InspectSession(t.Context(), token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("permanent grant extended login: %v", err)
	}
	var document string
	if err = db.SQL().QueryRow(`SELECT document FROM access_audit WHERE json_extract(document, '$.grant_id') = ?`, grant.ID).Scan(&document); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err = json.Unmarshal([]byte(document), &event); err != nil || event["permanent"] != true || event["expires_at_ms"] != nil {
		t.Fatalf("permanent audit record: %s %v", document, err)
	}
}

func TestPermanentControlRequiresConsentAndCanBeReplacedAndRevoked(t *testing.T) {
	store, _, admin, operator := newGrantFixture(t)
	if _, err := store.GrantPermanentControl(t.Context(), operator.ID, operator.ID); err == nil {
		t.Fatal("operator self-approved permanent control")
	}
	if _, err := store.GrantControl(t.Context(), admin.ID, operator.ID, 0); err == nil {
		t.Fatal("zero duration silently became permanent")
	}
	permanent, err := store.GrantPermanentControl(t.Context(), admin.ID, operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	timed, err := store.GrantControl(t.Context(), admin.ID, operator.ID, time.Hour)
	if err != nil || timed.ID == permanent.ID || timed.ExpiresAt == nil {
		t.Fatalf("permanent to timed replacement: %+v %v", timed, err)
	}
	permanent, err = store.GrantPermanentControl(t.Context(), admin.ID, operator.ID)
	if err != nil || permanent.ID == timed.ID || permanent.ExpiresAt != nil {
		t.Fatalf("timed to permanent replacement: %+v %v", permanent, err)
	}
	if err = store.RevokeControl(t.Context(), admin.ID, operator.ID); err != nil {
		t.Fatal(err)
	}
	if grant, readErr := store.ControlGrant(t.Context(), operator.ID); readErr != nil || grant != nil {
		t.Fatalf("permanent revocation: %+v %v", grant, readErr)
	}
}

func TestPermanentGrantStillRequiresEnabledAccounts(t *testing.T) {
	store, _, admin, operator := newGrantFixture(t)
	if _, err := store.GrantPermanentControl(t.Context(), admin.ID, operator.ID); err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetDisabled(t.Context(), operator.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectSession(t.Context(), token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("disabled permanent controller still authenticated: %v", err)
	}
	if _, err = store.GrantPermanentControl(t.Context(), admin.ID, operator.ID); err == nil {
		t.Fatal("disabled operator received permanent control")
	}
	if _, err = store.Create(t.Context(), "second-owner", "synthetic second owner passphrase", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err = store.SetDisabled(t.Context(), admin.ID, true); err != nil {
		t.Fatal(err)
	}
	if grant, readErr := store.ControlGrant(t.Context(), operator.ID); readErr != nil || grant != nil {
		t.Fatalf("disabled issuer retained permission: %+v %v", grant, readErr)
	}
}

func TestControlPermissionRequiresOwnerConsentAndExpires(t *testing.T) {
	db, err := appstore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := store.BootstrapAdmin(t.Context(), "owner", "a strong owner passphrase")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := store.Create(t.Context(), "observer", "a strong observer passphrase", RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.InspectSession(t.Context(), token)
	if err != nil || session.CanControl(time.Now()) {
		t.Fatalf("operator has implicit control: %+v %v", session, err)
	}
	if _, err := store.GrantControl(t.Context(), operator.ID, operator.ID, time.Hour); err == nil {
		t.Fatal("operator self-approved control")
	}
	grant, err := store.GrantControl(t.Context(), admin.ID, operator.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	session, err = store.InspectSession(t.Context(), token)
	if err != nil || !session.CanControl(time.Now()) || session.ControlGrant.ID != grant.ID {
		t.Fatalf("permission not applied: %v", err)
	}
	if session.CanControl(*grant.ExpiresAt) {
		t.Fatal("permission active at expiry boundary")
	}
	if err := store.RevokeControl(t.Context(), admin.ID, operator.ID); err != nil {
		t.Fatal(err)
	}
	session, err = store.InspectSession(t.Context(), token)
	if err != nil || session.CanControl(time.Now()) {
		t.Fatalf("revocation ended login or retained control: %v", err)
	}
}
