package persona

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func loreString(value string) *string { return &value }

func loreKeywords(values ...string) *[]string { return &values }

func loreEnabled(value bool) *bool { return &value }

func setLoreMode(t *testing.T, store *Store, personaID, mode string) {
	t.Helper()
	if _, err := store.Update(context.Background(), personaID, Draft{LoreMode: loreString(mode)}); err != nil {
		t.Fatalf("set lore mode %q: %v", mode, err)
	}
}

func TestLoreSelectionHonorsModesKeywordsAndEnabledState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")

	slow, err := store.CreateLore(ctx, item.ID, LoreDraft{
		Text:     loreString("Blue velvet is a familiar shared memory."),
		Keywords: loreKeywords("Velvet", "slow burn", "VELVET"),
	})
	if err != nil {
		t.Fatalf("create matching lore: %v", err)
	}
	boundary, err := store.CreateLore(ctx, item.ID, LoreDraft{
		Text:     loreString("A short keyword must still match a whole word."),
		Keywords: loreKeywords("he"),
	})
	if err != nil {
		t.Fatalf("create boundary lore: %v", err)
	}
	disabled, err := store.CreateLore(ctx, item.ID, LoreDraft{
		Text:     loreString("Disabled entries never enter a prompt."),
		Keywords: loreKeywords("rhythm"),
		Enabled:  loreEnabled(false),
	})
	if err != nil {
		t.Fatalf("create disabled lore: %v", err)
	}
	if got := strings.Join(slow.Keywords, ","); got != "velvet,slow burn" {
		t.Fatalf("normalized keywords = %q", got)
	}

	setLoreMode(t, store, item.ID, LoreModeRelevant)
	selection, err := store.SelectLore(ctx, item.ID, []string{"The rhythm should be a slow burn."})
	if err != nil {
		t.Fatalf("select relevant lore: %v", err)
	}
	if len(selection.EntryIDs) != 1 || selection.EntryIDs[0] != slow.ID {
		t.Fatalf("relevant selection = %v, want only %s", selection.EntryIDs, slow.ID)
	}
	if selection.Characters != len([]rune(slow.Text)) {
		t.Fatalf("selected characters = %d, want %d", selection.Characters, len([]rune(slow.Text)))
	}

	// "he" must not match inside "the"; short substring matches made Relevant
	// mode effectively Full for common words.
	if keywordMatch("the rhythm", "he") {
		t.Fatal("keyword matcher accepted a substring inside a word")
	}
	setLoreMode(t, store, item.ID, LoreModeFull)
	selection, err = store.SelectLore(ctx, item.ID, nil)
	if err != nil {
		t.Fatalf("select full lore: %v", err)
	}
	selected := make(map[string]bool, len(selection.EntryIDs))
	for _, id := range selection.EntryIDs {
		selected[id] = true
	}
	if len(selection.EntryIDs) != 2 || !selected[slow.ID] || !selected[boundary.ID] {
		t.Fatalf("full selection = %v, want enabled entries only", selection.EntryIDs)
	}
	for _, id := range selection.EntryIDs {
		if id == disabled.ID {
			t.Fatal("full mode selected a disabled entry")
		}
	}

	setLoreMode(t, store, item.ID, LoreModeOff)
	selection, err = store.SelectLore(ctx, item.ID, []string{"velvet"})
	if err != nil {
		t.Fatalf("select disabled lore: %v", err)
	}
	if len(selection.EntryIDs) != 0 || selection.Characters != 0 {
		t.Fatalf("off mode selected lore: %+v", selection)
	}
}

func TestKeywordMatchSupportsUnsegmentedScripts(t *testing.T) {
	for _, testCase := range []struct {
		text    string
		keyword string
	}{
		{text: "她记得蓝色天鹅绒的触感", keyword: "蓝色天鹅绒"},
		{text: "ゆっくりしたリズムを覚えている", keyword: "リズム"},
		{text: "부드러운리듬을기억해", keyword: "리듬"},
	} {
		if !keywordMatch(testCase.text, testCase.keyword) {
			t.Fatalf("keyword %q did not match %q", testCase.keyword, testCase.text)
		}
	}
}

func TestLoreRejectsRowAggregateAndEntryCountOverflow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")

	for label, draft := range map[string]LoreDraft{
		"empty":         {Text: loreString("   ")},
		"long text":     {Text: loreString(strings.Repeat("x", MaxLoreTextChars+1))},
		"many keywords": {Text: loreString("valid"), Keywords: loreKeywords(makeStrings(MaxLoreKeywords+1, "tag")...)},
		"long keyword":  {Text: loreString("valid"), Keywords: loreKeywords(strings.Repeat("k", MaxLoreKeywordChars+1))},
	} {
		if _, err := store.CreateLore(ctx, item.ID, draft); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: err = %v, want ErrInvalid", label, err)
		}
	}

	for index := 0; index < MaxLoreTotalChars/MaxLoreTextChars; index++ {
		text := strings.Repeat(string(rune('a'+index)), MaxLoreTextChars)
		if _, err := store.CreateLore(ctx, item.ID, LoreDraft{Text: &text}); err != nil {
			t.Fatalf("fill aggregate budget at %d: %v", index, err)
		}
	}
	overflow := "x"
	if _, err := store.CreateLore(ctx, item.ID, LoreDraft{Text: &overflow}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("aggregate overflow err = %v, want ErrInvalid", err)
	}

	other := createPersona(t, store, "Mara")
	for index := 0; index < MaxLoreEntries; index++ {
		text := "entry " + string(rune('a'+index))
		if _, err := store.CreateLore(ctx, other.ID, LoreDraft{Text: &text}); err != nil {
			t.Fatalf("create entry %d: %v", index, err)
		}
	}
	if _, err := store.CreateLore(ctx, other.ID, LoreDraft{Text: loreString("one too many")}); !errors.Is(err, ErrLimit) {
		t.Fatalf("entry overflow err = %v, want ErrLimit", err)
	}
}

func TestDuplicateCopiesLoreAndDeleteCascadesIt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	source := createPersona(t, store, "Rowan")
	setLoreMode(t, store, source.ID, LoreModeRelevant)
	entry, err := store.CreateLore(ctx, source.ID, LoreDraft{
		Text:     loreString("A shared reference."),
		Keywords: loreKeywords("reference"),
	})
	if err != nil {
		t.Fatalf("create lore: %v", err)
	}

	copied, err := store.Duplicate(ctx, source.ID)
	if err != nil {
		t.Fatalf("duplicate persona: %v", err)
	}
	if copied.LoreMode != LoreModeRelevant || copied.LoreCount != 1 {
		t.Fatalf("duplicate lore summary = mode %q count %d", copied.LoreMode, copied.LoreCount)
	}
	entries, err := store.ListLore(ctx, copied.ID)
	if err != nil {
		t.Fatalf("list copied lore: %v", err)
	}
	if len(entries) != 1 || entries[0].ID == entry.ID || entries[0].Text != entry.Text {
		t.Fatalf("copied lore = %+v, source = %+v", entries, entry)
	}

	if err := store.Delete(ctx, copied.ID); err != nil {
		t.Fatalf("delete duplicate: %v", err)
	}
	var count int
	if err := store.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM persona_lore WHERE persona_id = ?`, copied.ID).Scan(&count); err != nil {
		t.Fatalf("count cascaded lore: %v", err)
	}
	if count != 0 {
		t.Fatalf("deleted persona left %d lore rows", count)
	}
}

func TestUnknownPersistedLoreModeFailsClosed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")
	if _, err := store.CreateLore(ctx, item.ID, LoreDraft{Text: loreString("Must remain out of the prompt.")}); err != nil {
		t.Fatalf("create lore: %v", err)
	}
	if _, err := store.db.SQL().ExecContext(ctx,
		`UPDATE personas SET lore_mode = 'future-mode' WHERE id = ?`, item.ID); err != nil {
		t.Fatalf("seed unknown mode: %v", err)
	}
	selection, err := store.SelectLore(ctx, item.ID, []string{"anything"})
	if err != nil {
		t.Fatalf("select lore: %v", err)
	}
	if len(selection.EntryIDs) != 0 {
		t.Fatalf("unknown mode selected lore: %+v", selection)
	}
}

func makeStrings(count int, prefix string) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = prefix + string(rune('a'+index))
	}
	return values
}
