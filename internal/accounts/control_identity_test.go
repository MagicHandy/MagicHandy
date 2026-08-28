package accounts

import (
	"errors"
	"testing"
)

func TestControlIdentityIsPerSessionAndRequiresActiveLink(t *testing.T) {
	store, database := newAccountStore(t)
	owner, err := store.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	linked, err := store.Create(t.Context(), "partner", "another excellent password", RoleOperator)
	if err != nil {
		t.Fatalf("Create linked account: %v", err)
	}
	unlinked, err := store.Create(t.Context(), "other", "yet another excellent password", RoleOperator)
	if err != nil {
		t.Fatalf("Create unlinked account: %v", err)
	}
	if _, err := database.SQL().Exec(`
		INSERT INTO user_account_links(owner_user_id, linked_user_id, label, status, created_at, updated_at)
		VALUES(?, ?, 'Partner device', 'active', 'now', 'now')
	`, owner.ID, linked.ID); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	token, session, err := store.NewSession(t.Context(), owner.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if session.ControlAccountID != owner.ID {
		t.Fatalf("initial control account = %q, want self %q", session.ControlAccountID, owner.ID)
	}
	identities, err := store.ControlIdentities(t.Context(), owner.ID, session.ControlAccountID)
	if err != nil {
		t.Fatalf("ControlIdentities: %v", err)
	}
	if len(identities) != 2 || !identities[0].Selected || identities[0].Relationship != ControlRelationshipSelf ||
		identities[1].Label != "Partner device" || identities[1].Selected {
		t.Fatalf("initial identities = %+v", identities)
	}

	if err := store.SetControlIdentity(t.Context(), token, linked.ID); err != nil {
		t.Fatalf("SetControlIdentity linked: %v", err)
	}
	resolved, err := store.ResolveSession(t.Context(), token)
	if err != nil {
		t.Fatalf("ResolveSession: %v", err)
	}
	if resolved.ControlAccountID != linked.ID {
		t.Fatalf("selected control account = %q, want %q", resolved.ControlAccountID, linked.ID)
	}
	if err := store.SetControlIdentity(t.Context(), token, unlinked.ID); !errors.Is(err, ErrControlIdentityNotAllowed) {
		t.Fatalf("unlinked selection error = %v", err)
	}

	secondToken, _, err := store.NewSession(t.Context(), owner.ID)
	if err != nil {
		t.Fatalf("second session: %v", err)
	}
	second, err := store.ResolveSession(t.Context(), secondToken)
	if err != nil || second.ControlAccountID != owner.ID {
		t.Fatalf("second session inherited selection: session=%+v err=%v", second, err)
	}
}

func TestControlIdentityFallsBackToSelfWhenLinkIsNoLongerActive(t *testing.T) {
	store, database := newAccountStore(t)
	owner, err := store.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	linked, err := store.Create(t.Context(), "partner", "another excellent password", RoleOperator)
	if err != nil {
		t.Fatalf("Create linked account: %v", err)
	}
	if _, err := database.SQL().Exec(`
		INSERT INTO user_account_links(owner_user_id, linked_user_id, label, status, created_at, updated_at)
		VALUES(?, ?, '', 'active', 'now', 'now')
	`, owner.ID, linked.ID); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	token, _, err := store.NewSession(t.Context(), owner.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := store.SetControlIdentity(t.Context(), token, linked.ID); err != nil {
		t.Fatalf("SetControlIdentity: %v", err)
	}
	if _, err := database.SQL().Exec(`
		UPDATE user_account_links SET status = 'revoked' WHERE owner_user_id = ? AND linked_user_id = ?
	`, owner.ID, linked.ID); err != nil {
		t.Fatalf("revoke link: %v", err)
	}
	resolved, err := store.ResolveSession(t.Context(), token)
	if err != nil {
		t.Fatalf("ResolveSession after revoke: %v", err)
	}
	if resolved.ControlAccountID != owner.ID {
		t.Fatalf("resolved revoked control account = %q, want self %q", resolved.ControlAccountID, owner.ID)
	}
	identities, err := store.ControlIdentities(t.Context(), owner.ID, linked.ID)
	if err != nil {
		t.Fatalf("ControlIdentities: %v", err)
	}
	if len(identities) != 1 || !identities[0].Selected || identities[0].Relationship != ControlRelationshipSelf {
		t.Fatalf("fallback identities = %+v", identities)
	}
}

func TestProfileVisibilityRequiresAnActiveLinkToAnEnabledAccount(t *testing.T) {
	store, database := newAccountStore(t)
	if _, err := store.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple"); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	viewer, err := store.Create(t.Context(), "viewer", "another excellent password", RoleOperator)
	if err != nil {
		t.Fatalf("Create viewer: %v", err)
	}
	target, err := store.Create(t.Context(), "partner", "yet another excellent password", RoleOperator)
	if err != nil {
		t.Fatalf("Create target: %v", err)
	}
	if _, err := database.SQL().Exec(`
		INSERT INTO user_account_links(owner_user_id, linked_user_id, label, status, created_at, updated_at)
		VALUES(?, ?, '', 'active', 'now', 'now')
	`, viewer.ID, target.ID); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	allowed, err := store.CanViewProfile(t.Context(), viewer, target.ID)
	if err != nil || !allowed {
		t.Fatalf("active visibility = %t, %v", allowed, err)
	}
	if err := store.SetDisabled(t.Context(), target.ID, true); err != nil {
		t.Fatalf("disable target: %v", err)
	}
	allowed, err = store.CanViewProfile(t.Context(), viewer, target.ID)
	if err != nil || allowed {
		t.Fatalf("disabled visibility = %t, %v", allowed, err)
	}
}
