package accounts

import (
	"testing"
	"time"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
)

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
	if session.CanControl(grant.ExpiresAt) {
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
