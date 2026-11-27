package chat

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestSavedHistoryBindsOriginalTurnsAndBoundsWholeMessages(t *testing.T) {
	log, id := recoveryLog(t)
	first := appendRecoveryMessage(t, log, "My checkpoint is Bluebell.")
	if _, err := log.AppendTo(id, MessageRoleAssistant, "invented checkpoint", "", nil); err != nil {
		t.Fatal(err)
	}
	appendRecoveryMessage(t, log, strings.Repeat("界", lookupByteLimit))
	late := appendRecoveryMessage(t, log, "checkpoint with the corrected baseline")
	current := appendRecoveryMessage(t, log, "What checkpoint and baseline did I name?")
	appendRecoveryMessage(t, log, "future checkpoint and baseline")
	if _, err := log.SaveSession(id); err != nil {
		t.Fatal(err)
	}
	if _, err := log.CreateSession(false); err != nil {
		t.Fatal(err)
	}
	appendRecoveryMessage(t, log, "other chat checkpoint and baseline")
	read := log.UserHistoryBefore(id, current)
	q := HistoryLookup{Action: "read_user_history", Scope: "opening", Terms: []string{}, Facts: []historyFact{{"word", "text"}}}
	got, err := read(t.Context(), q)
	if err != nil || len(got) != 1 || got[0].ID != first || got[0].Text != "My checkpoint is Bluebell." {
		t.Fatalf("opening crossed an oversized turn or used another source: %v, %v", got, err)
	}
	q.Scope, q.Terms = "matching", []string{"checkpoint", "baseline"}
	got, err = read(t.Context(), q)
	if err != nil || len(got) != 2 || got[0].ID != late || got[1].ID != first {
		t.Fatalf("ranked lookup crossed its session, role or snapshot: %v, %v", got, err)
	}
	if err := log.Delete(first); err != nil {
		t.Fatal(err)
	}
	got, err = read(t.Context(), q)
	if err != nil || len(got) != 1 || got[0].ID != late {
		t.Fatalf("surviving named evidence: %v, %v", got, err)
	}
	q.Scope, q.Terms = "opening", []string{}
	got, err = read(t.Context(), q)
	if err != nil || len(got) != 0 {
		t.Fatal("a deleted opening was replaced by later evidence", got, err)
	}
}

func TestSavedHistoryCannotReconstructAPrunedBeginning(t *testing.T) {
	log, id := recoveryLog(t)
	if _, err := log.db.SQL().Exec(`WITH RECURSIVE n(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i <= ?)
		INSERT INTO messages(session_id, role, content, created_at, committed, revision)
		SELECT ?, 'user', 'old detail', '2026-11-27T00:00:00Z', 1, i FROM n`, MessageRetentionCap, id); err != nil {
		t.Fatal(err)
	}
	head := appendRecoveryMessage(t, log, "recall question")
	lookup := HistoryLookup{Action: "read_user_history", Scope: "opening", Terms: []string{}, Facts: []historyFact{{"initial detail", "text"}}}
	got, err := log.UserHistoryBefore(id, head)(t.Context(), lookup)
	if err != nil || len(got) != 0 {
		t.Fatal("pruned conversation offered a replacement opening", got, err)
	}
}

func TestSavedHistoryAnswerRequiresSourceSpansAndNeverCarriesMotion(t *testing.T) {
	const lookup = `{"action":"read_user_history","scope":"opening","terms":[],"facts":[{"detail":"first pace","kind":"number"},{"detail":"next pace","kind":"number"},{"detail":"increase","kind":"number"},{"detail":"word","kind":"text"}]}`
	p := &scriptedProvider{responses: []string{lookup, `{"fact1":{"source":7,"quote":"25 percent"},"fact2":{"source":7,"quote":"25"},"fact3":{"source":8,"quote":"five percentage points"},"fact4":{"source":99,"quote":"unrelated"}}`}}
	service := Service{Provider: p, Capabilities: &Capabilities{}, PromptBudget: PromptBudgetSettings{ContextSize: 16384}}
	request := Request{Message: "What did I ask for first?", HistoryMessagesDropped: 3, ReadUserHistory: func(context.Context, HistoryLookup) ([]HistorySource, error) {
		return []HistorySource{{7, "Set 25 percent. Begin moving now."}, {8, "Five percentage points faster."}}, nil
	}}
	got, err := service.Complete(t.Context(), request, nil)
	if err != nil || got.Response.Reply != "25 percent, Unavailable, Five percentage points, Unavailable" || got.Response.Motion != nil || got.Repaired || len(p.requests) != 2 {
		t.Fatalf("unverified output or motion on lookup: %+v, %v", got, err)
	}
	for i, sent := range p.requests {
		data, _ := json.Marshal(sent.Messages)
		if strings.Contains(string(data), "Begin moving now") != (i == 1) {
			t.Fatal("archived directive escaped the read-only evidence request")
		}
		if _, _, err := llm.BudgetChatRequest(sent, 16384); err != nil {
			t.Fatal(err)
		}
	}
	if claimHistoryQuote([]HistorySource{{7, "Set 25 percent"}}, 7, "5", map[int64][]quoteSpan{}) != "" {
		t.Fatal("a substring of a different number was accepted")
	}
	p.responses = []string{strings.Replace(lookup, `"terms":[]`, `"terms":[],"motion":{"action":"start"}`, 1)}
	if result, err := service.Complete(t.Context(), request, nil); err == nil || result.Response.Motion != nil {
		t.Fatal("mixed recall and motion envelope was accepted")
	}
}

func TestHistoryLookupHonorsBudgetsAndOrdinaryTurns(t *testing.T) {
	read := func(context.Context, HistoryLookup) ([]HistorySource, error) {
		t.Fatal("ordinary turn read the archive")
		return nil, nil
	}
	for _, local := range []bool{true, false} {
		p := &scriptedProvider{}
		service := Service{Provider: p, Capabilities: &Capabilities{LocalInference: local, SimpleMotion: local}}
		// A wholly visible chat keeps its existing prompt, schema and call count.
		p.responses = []string{`{"reply":"Ready"}`}
		req := Request{Message: "Ready?", History: []llm.Message{{Role: "user", Content: "Hello"}}, ReadUserHistory: read}
		if _, err := service.Complete(t.Context(), req, nil); err != nil || len(p.requests) != 1 || strings.Contains(string(p.requests[0].JSONSchema), "read_user_history") {
			t.Fatal("short chat changed", err)
		}
		req.HistoryMessagesDropped = 5
		p.responses = []string{`{"reply":"Ready"}`}
		if local {
			p.responses = slices.Insert(p.responses, 0, `{"ordinary":true}`)
		}
		result, err := service.Complete(t.Context(), req, nil)
		if err != nil || result.Response.Reply != "Ready" || result.RepairAttempted {
			t.Fatal("ordinary chat lost its usual completion", result, err)
		}
	}
}

func TestOpeningPaceEvidenceStopsAtUnverifiedOrReplacedSetting(t *testing.T) {
	source := []HistorySource{{11, "Set 25 percent"}, {13, "Set 35 percent"}, {15, "Five points faster"}}
	v25, v35, v5 := 25.0, 35.0, 5.0
	a := citedPace{"set", &v25, "25 percent"}
	b := citedPace{"set", &v35, "35 percent"}
	delta := citedPace{"adjust", &v5, "Five points"}
	keys := []string{"message11", "message13", "message15"}
	rows := map[string][]citedPace{keys[0]: {a}, keys[1]: {b}, keys[2]: {delta}}
	events, intact, err := verifiedPacePrefix(source, keys, rows)
	if err != nil || !intact || renderOpeningPaces(events, 2, true, "?") != "25 percent, 35 percent, Five points" {
		t.Fatal(events, err)
	}
	for _, bad := range []citedPace{{"set", &v35, "not in the message"}, {"set", &v25, "35 percent"}} {
		rows[keys[1]] = []citedPace{bad}
		events, intact, err = verifiedPacePrefix(source, keys, rows)
		if err != nil || intact || renderOpeningPaces(events, 2, true, "?") != "25 percent, ?, ?" {
			t.Fatal("later event crossed unverified evidence", events, err)
		}
	}
	if got := renderOpeningPaces([]citedPace{a, b, a, delta}, 2, true, "?"); got != "25 percent, 35 percent, ?" {
		t.Fatal("a later adjustment was attributed to a superseded setting", got)
	}
}
