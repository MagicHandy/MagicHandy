package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testStore(t *testing.T) (*Store, *appstore.DB) {
	t.Helper()
	db, err := appstore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db), db
}

func TestAuditRejectsUnreviewedContentAndReferences(t *testing.T) {
	s, _ := testStore(t)
	for _, event := range []Event{
		{Kind: Kind("private error detail"), Outcome: "success"},
		{Kind: StopFinished, Outcome: "private failure"},
		{Kind: StopFinished, Outcome: "success", Operation: "C:/private/worker.exe"},
		{Kind: StopFinished, Outcome: "success", Actor: Actor{Type: "account", AccountID: "private-token"}},
		{Kind: StopFinished, Outcome: "success", Correlation: "http://private-service.invalid"},
	} {
		if err := s.Append(t.Context(), []Event{event}); err == nil {
			t.Fatal("unreviewed content entered durable history")
		}
	}
	page, err := s.Page(t.Context(), 0)
	if err != nil || len(page.Events) != 0 {
		t.Fatalf("rejected events persisted: %+v %v", page, err)
	}
}

func TestAuditReadRejectsMissingStoredAttribution(t *testing.T) {
	s, db := testStore(t)
	if _, err := db.SQL().Exec(`INSERT INTO access_audit(event_id,occurred_at,document) VALUES('corrupt-fixture',?,?)`, time.Now().UnixMilli(), `{"kind":"stop_finished","outcome":"success"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Page(t.Context(), 0); err == nil {
		t.Fatal("read invented an identity and timestamp for a corrupt event")
	}
}

func TestAuditWriteRollsBackWithItsOwningTransaction(t *testing.T) {
	s, db := testStore(t)
	rollback := errors.New("rollback owning mutation")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := AppendTx(t.Context(), tx, Event{Kind: GrantIssued, Outcome: "success"}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	page, err := s.Page(t.Context(), 0)
	if err != nil || len(page.Events) != 0 {
		t.Fatal("rolled-back change retained an audit success")
	}
}

func TestAuditBoundsRowsAndPaginatesWithoutHoldingTheWriter(t *testing.T) {
	s, db := testStore(t)
	batch := make([]Event, appstore.AuditRowLimit+7)
	for i := range batch {
		batch[i] = Event{Kind: CommandFinished, Outcome: "success", Operation: "motion"}
	}
	if err := s.Append(t.Context(), batch); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT count(*) FROM access_audit`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != appstore.AuditRowLimit {
		t.Fatalf("retained %d rows", count)
	}
	first, err := s.Page(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Page(t.Context(), first.NextBefore)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != PageSize || !first.HasMore || len(second.Events) != PageSize || second.Events[0].Sequence >= first.NextBefore {
		t.Fatal("bounded history page did not advance")
	}
	data, err := json.Marshal(first)
	if err != nil || len(data) > PageMaxBytes {
		t.Fatalf("page exceeded byte budget: %d %v", len(data), err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := s.Append(ctx, []Event{{Kind: StopFinished, Outcome: "success"}}); err != nil {
		t.Fatal("page retained database work after return:", err)
	}
}

func TestAuditExpiryAndRepeatedCommittedBatch(t *testing.T) {
	s, db := testStore(t)
	old := Event{Kind: ServerStarted, Outcome: "success", OccurredAt: time.Now().Add(-31 * 24 * time.Hour).UnixMilli()}
	event, _, err := prepare(Event{Kind: StopFinished, Outcome: "unconfirmed", Correlation: CorrelateCommand("private request identifier")})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(t.Context(), []Event{old, event}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(t.Context(), []Event{event}); err != nil {
		t.Fatal(err)
	}
	page, err := s.Page(t.Context(), 0)
	if err != nil || len(page.Events) != 1 || page.Events[0].ID != event.ID {
		t.Fatal("expiry or idempotent batch failed", err)
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT count(*) FROM access_audit`).Scan(&count); err != nil || count != 1 {
		t.Fatal("expired data remained durable", count, err)
	}
	data, err := json.Marshal(page)
	if err != nil || strings.Contains(string(data), "private request identifier") {
		t.Fatal("raw request identifier was retained")
	}
}
