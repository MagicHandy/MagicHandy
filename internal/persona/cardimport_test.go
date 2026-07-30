package persona

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/charcard"
)

func TestImportCardCreatesPersonaWithLoreAndGreeting(t *testing.T) {
	store := newTestStore(t)
	card := charcard.Card{
		Name:        "Annabelle",
		Description: "A shy step-sister of {{user}}.",
		Personality: "Curious, stubborn.",
		Scenario:    "Late night in the kitchen with {{char}}.",
		Greeting:    "*{{char}} looks up at {{user}}.* Oh, it's you.",
	}
	item, warnings, err := store.ImportCard(context.Background(), card, nil)
	if err != nil {
		t.Fatalf("ImportCard: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a small card", warnings)
	}
	if item.Name != "Annabelle" {
		t.Fatalf("name = %q", item.Name)
	}
	if strings.Contains(item.Description, "{{user}}") {
		t.Fatalf("description macros not replaced: %q", item.Description)
	}
	if item.Greeting != "*Annabelle looks up at you.* Oh, it's you." {
		t.Fatalf("greeting = %q", item.Greeting)
	}
	if item.LoreMode != LoreModeFull {
		t.Fatalf("lore mode = %q", item.LoreMode)
	}
	entries, err := store.ListLore(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("list lore: %v", err)
	}
	var texts []string
	for _, entry := range entries {
		texts = append(texts, entry.Text)
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "Curious, stubborn.") {
		t.Fatalf("personality missing from lore: %q", joined)
	}
	if !strings.Contains(joined, "Late night in the kitchen with Annabelle.") {
		t.Fatalf("scenario missing from lore (or macros kept): %q", joined)
	}
}

func TestImportCardRespectsBoundsAndReportsTruncation(t *testing.T) {
	store := newTestStore(t)
	card := charcard.Card{
		Name:            strings.Repeat("N", MaxNameChars+40),
		Description:     strings.Repeat("d ", 2000),
		Personality:     strings.Repeat("p ", 2000),
		Scenario:        strings.Repeat("s ", 2000),
		ExampleMessages: strings.Repeat("e ", 2000),
		Greeting:        strings.Repeat("g ", MaxGreetingChars),
	}
	item, warnings, err := store.ImportCard(context.Background(), card, nil)
	if err != nil {
		t.Fatalf("ImportCard: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected truncation warnings")
	}
	if utf8.RuneCountInString(item.Name) > MaxNameChars {
		t.Fatalf("name too long: %d runes", utf8.RuneCountInString(item.Name))
	}
	if utf8.RuneCountInString(item.Description) > MaxDescriptionChars {
		t.Fatalf("description too long: %d runes", utf8.RuneCountInString(item.Description))
	}
	if utf8.RuneCountInString(item.Greeting) > MaxGreetingChars {
		t.Fatalf("greeting too long: %d runes", utf8.RuneCountInString(item.Greeting))
	}
	entries, err := store.ListLore(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("list lore: %v", err)
	}
	if len(entries) == 0 || len(entries) > MaxLoreEntries {
		t.Fatalf("lore entries = %d", len(entries))
	}
	total := 0
	for _, entry := range entries {
		if utf8.RuneCountInString(entry.Text) > MaxLoreTextChars {
			t.Fatalf("lore entry too long: %d runes", utf8.RuneCountInString(entry.Text))
		}
		total += utf8.RuneCountInString(entry.Text)
	}
	if total > MaxLoreTotalChars {
		t.Fatalf("lore total = %d runes", total)
	}
}

func TestImportCardConvertsPortraitToBoundedJPEG(t *testing.T) {
	store := newTestStore(t)
	canvas := image.NewRGBA(image.Rect(0, 0, 1600, 900))
	var art bytes.Buffer
	if err := png.Encode(&art, canvas); err != nil {
		t.Fatalf("encode art: %v", err)
	}
	card := charcard.Card{Name: "Lily", Greeting: "Hallo!"}
	item, _, err := store.ImportCard(context.Background(), card, art.Bytes())
	if err != nil {
		t.Fatalf("ImportCard: %v", err)
	}
	if !item.HasPortrait {
		t.Fatal("expected a portrait")
	}
	file, err := store.OpenPortrait(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("open portrait: %v", err)
	}
	defer func() { _ = file.Close() }()
	header, err := jpeg.DecodeConfig(file)
	if err != nil {
		t.Fatalf("portrait is not a JPEG: %v", err)
	}
	if header.Width > MaxPortraitEdge || header.Height > MaxPortraitEdge {
		t.Fatalf("portrait %dx%d exceeds edge %d", header.Width, header.Height, MaxPortraitEdge)
	}
}

func TestImportCardWithBrokenArtStillImports(t *testing.T) {
	store := newTestStore(t)
	card := charcard.Card{Name: "NoArt"}
	item, warnings, err := store.ImportCard(context.Background(), card, []byte("not a png"))
	if err != nil {
		t.Fatalf("ImportCard: %v", err)
	}
	if item.HasPortrait {
		t.Fatal("broken art must not produce a portrait")
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning about unusable art")
	}
}
