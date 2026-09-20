package notices

import (
	"database/sql"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/store"
)

func noticeDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, id := range []string{"alice", "bob"} {
		_, err := db.SQL().Exec(`INSERT INTO user_accounts(id, username, username_key, role, password_hash, created_at, updated_at) VALUES(?,?,?,'operator','synthetic hash','created','updated')`, id, id, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func saveNotice(t *testing.T, db *store.DB, owner Owner, id string, now time.Time) {
	t.Helper()
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return SetHiddenTx(t.Context(), tx, owner, id, true, now) }); err != nil {
		t.Fatal(err)
	}
}

func TestPreferencesFollowAccountsButSignInNoticeBelongsToBrowser(t *testing.T) {
	db := noticeDB(t)
	now := time.Now()
	a := Owner{AccountID: "alice", BrowserHash: strings.Repeat("a", 64)}
	b := Owner{AccountID: "alice", BrowserHash: strings.Repeat("b", 64)}
	saveNotice(t, db, a, "model-generation", now)
	saveNotice(t, db, a, "sign-in-safety", now)
	for _, test := range []struct {
		owner Owner
		want  []string
		scope string
	}{
		{a, []string{"sign-in-safety", "model-generation"}, "account"},
		{b, []string{"model-generation"}, "account"},
		{Owner{BrowserHash: a.BrowserHash}, []string{"sign-in-safety"}, "browser"},
		{Owner{AccountID: "bob", BrowserHash: b.BrowserHash}, []string{}, "account"},
	} {
		got, err := Read(t.Context(), db.SQL(), test.owner, now)
		if err != nil || got.Scope != test.scope || !slices.Equal(got.Hidden, test.want) {
			t.Fatalf("scope isolation: %+v %v", got, err)
		}
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return ResetTx(t.Context(), tx, b) }); err != nil {
		t.Fatal(err)
	}
	got, err := Read(t.Context(), db.SQL(), a, now)
	if err != nil || !slices.Equal(got.Hidden, []string{"sign-in-safety"}) {
		t.Fatalf("account reset crossed browser boundary: %+v %v", got, err)
	}
}

func TestConcurrentNoticeChangesPreserveBothChoicesAndSurviveReopen(t *testing.T) {
	db := noticeDB(t)
	owner := Owner{AccountID: "alice", BrowserHash: strings.Repeat("c", 64)}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, id := range []string{"model-generation", "device-firmware"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- db.WithTx(t.Context(), func(tx *sql.Tx) error { return SetHiddenTx(t.Context(), tx, owner, id, true, time.Now()) })
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	dir := db.DataDir()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	got, err := Read(t.Context(), reopened.SQL(), owner, time.Now())
	if err != nil || !slices.Equal(got.Hidden, []string{"model-generation", "device-firmware"}) {
		t.Fatalf("concurrent saved choices lost: %+v %v", got, err)
	}
}

func TestBrowserPreferencesExpireAndUnknownNoticesCannotBeHidden(t *testing.T) {
	db := noticeDB(t)
	now := time.Now()
	owner := Owner{AccountID: "alice", BrowserHash: strings.Repeat("d", 64)}
	saveNotice(t, db, owner, "sign-in-safety", now)
	saveNotice(t, db, owner, "model-generation", now)
	got, err := Read(t.Context(), db.SQL(), owner, now.Add(BrowserLifetime+time.Second))
	if err != nil || !slices.Equal(got.Hidden, []string{"model-generation"}) {
		t.Fatalf("unexpected expiry: %+v %v", got, err)
	}
	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return SetHiddenTx(t.Context(), tx, owner, "emergency-stop-control", true, now)
	})
	if err != ErrUnknown {
		t.Fatalf("unknown notice accepted: %v", err)
	}
}
