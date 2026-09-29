package accounts

import (
	"errors"
	"testing"
	"time"
)

func TestInterfaceDowngradeRevokesSessionsAndGrant(t *testing.T) {
	s, _ := newAccountStore(t)
	admin, err := s.BootstrapAdmin(t.Context(), "owner", "synthetic administrator passphrase")
	if err != nil {
		t.Fatal(err)
	}
	_, actor, err := s.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.Create(t.Context(), "member", "synthetic operator passphrase", RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GrantPermanentControl(t.Context(), admin.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	token, login, err := s.NewSession(t.Context(), member.ID)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := s.SetInterfaceAccessForSession(t.Context(), actor.Key, member.ID, InterfaceRemote)
	if err != nil || len(keys) != 1 || keys[0] != login.Key {
		t.Fatalf("downgrade: %v %v", keys, err)
	}
	if _, err = s.ResolveSession(t.Context(), token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("old token survived: %v", err)
	}
	if grant, err := s.ControlGrant(t.Context(), member.ID); err != nil || grant != nil {
		t.Fatalf("old grant survived: %v %v", grant, err)
	}
	if _, _, err = s.LoginWithClient(t.Context(), "member", "synthetic operator passphrase", SessionClient{}); !errors.Is(err, ErrInterfaceAccess) {
		t.Fatalf("main login admitted: %v", err)
	}
	remoteToken, _, err := s.LoginForInterface(t.Context(), "member", "synthetic operator passphrase", SessionClient{}, InterfaceRemote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GrantControl(t.Context(), admin.ID, member.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	remote, err := s.ResolveSession(t.Context(), remoteToken)
	if err != nil || remote.CanControl(time.Now()) || !remote.CanUseRemote(time.Now()) || remote.RemoteDesktopAccount() != admin.ID {
		t.Fatalf("remote authority incorrect: %+v %v", remote, err)
	}
	if _, err = s.SetInterfaceAccessForSession(t.Context(), actor.Key, admin.ID, InterfaceRemote); !errors.Is(err, ErrInterfaceAccess) {
		t.Fatalf("administrator downgraded to remote: %v", err)
	}
}

func TestRemoteLoginDoesNotInheritAdministratorControl(t *testing.T) {
	s, _ := newAccountStore(t)
	admin, err := s.BootstrapAdmin(t.Context(), "owner", "synthetic administrator passphrase")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := s.LoginForInterface(t.Context(), admin.Username, "synthetic administrator passphrase", SessionClient{}, InterfaceRemote)
	if err != nil {
		t.Fatal(err)
	}
	login, err := s.ResolveSession(t.Context(), token)
	if err != nil || login.CanControl(time.Now()) || !login.CanUseRemote(time.Now()) {
		t.Fatalf("remote admin control: %+v %v", login, err)
	}
	if _, err := s.CreateForSession(t.Context(), login.Key, "forbidden", "synthetic forbidden password", RoleOperator); !errors.Is(err, ErrAdministratorRequired) {
		t.Fatalf("remote login created an account: %v", err)
	}
}

func TestNewPasswordMinimumPreservesOlderLogins(t *testing.T) {
	s, db := newAccountStore(t)
	admin, err := s.BootstrapAdmin(t.Context(), "owner", "synthetic administrator passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePassword("eight888"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatal("new short password accepted")
	}
	salt := []byte("synthetic-salt-16")
	legacy := encodePasswordHash(currentPasswordParameters, salt, derivePassword([]byte("eight888"), salt, currentPasswordParameters))
	if _, err = db.SQL().Exec(`UPDATE user_accounts SET password_hash = ? WHERE id = ?`, legacy, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.LoginWithClient(t.Context(), admin.Username, "eight888", SessionClient{}); err != nil {
		t.Fatalf("legacy password no longer works: %v", err)
	}
}

func TestControlIdentityRejectsExpiredSession(t *testing.T) {
	s, _ := newAccountStore(t)
	admin, err := s.BootstrapAdmin(t.Context(), "owner", "synthetic administrator passphrase")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := s.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	later := s.now().Add(DefaultSessionLifetime + time.Hour)
	s.now = func() time.Time { return later }
	if err = s.SetControlIdentity(t.Context(), token, admin.ID); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired attribution change: %v", err)
	}
}
