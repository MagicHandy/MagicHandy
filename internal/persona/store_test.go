package persona

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := dbstore.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open datastore: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := OpenWithDatabase(database)
	if err != nil {
		t.Fatalf("open persona store: %v", err)
	}
	return store
}

func name(value string) *string { return &value }

func createPersona(t *testing.T, store *Store, displayName string) Persona {
	t.Helper()
	item, err := store.Create(context.Background(), Draft{Name: name(displayName)})
	if err != nil {
		t.Fatalf("create %q: %v", displayName, err)
	}
	return item
}

// jpegOfSize builds a real JPEG so the store's decode check is exercised rather
// than only its magic-byte prefix check.
func jpegOfSize(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 0x40, A: 0xFF})
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, canvas, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode fixture JPEG: %v", err)
	}
	return buffer.Bytes()
}

func TestCreateResolvesDefaultsFromANameAlone(t *testing.T) {
	store := newTestStore(t)

	item := createPersona(t, store, "  Rowan   Vale ")

	// The name is space-collapsed rather than rejected: a pasted name with odd
	// whitespace is a normal input, not an error worth blocking creation on.
	if item.Name != "Rowan Vale" {
		t.Fatalf("name = %q, want collapsed whitespace", item.Name)
	}
	if item.ChatVoice != config.LLMChatVoiceWarm {
		t.Fatalf("chat voice = %q, want the warm default", item.ChatVoice)
	}
	if item.ReactionStyle != config.LLMReactionStyleNeutral {
		t.Fatalf("reaction style = %q, want neutral so the prompt is unchanged by default", item.ReactionStyle)
	}
	if item.DefaultFocusArea != chat.AreaZoneFull {
		t.Fatalf("starting zone = %q, want the full range", item.DefaultFocusArea)
	}
	if item.HasPortrait {
		t.Fatal("a new persona must not claim a portrait it has never been given")
	}
	if !ValidID(item.ID) {
		t.Fatalf("minted ID %q fails its own validator", item.ID)
	}
}

func TestCreateRejectsValuesThisBuildCannotCompose(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for label, draft := range map[string]Draft{
		"empty name":     {Name: name("   ")},
		"long name":      {Name: name(strings.Repeat("a", MaxNameChars+1))},
		"long summary":   {Name: name("Rowan"), Description: name(strings.Repeat("b", MaxDescriptionChars+1))},
		"unknown voice":  {Name: name("Rowan"), ChatVoice: name("seductive")},
		"unknown style":  {Name: name("Rowan"), ReactionStyle: name("bratty")},
		"unknown zone":   {Name: name("Rowan"), DefaultFocusArea: name("everywhere")},
		"long prompt id": {Name: name("Rowan"), PromptSetID: name(strings.Repeat("c", MaxPromptSetIDChars+1))},
	} {
		if _, err := store.Create(ctx, draft); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: err = %v, want ErrInvalid", label, err)
		}
	}
}

func TestUpdateOmittedFieldsPreserveSavedValues(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")

	configured, err := store.Update(ctx, item.ID, Draft{
		ChatVoice:     name(config.LLMChatVoiceIntimate),
		ReactionStyle: name(config.LLMReactionStyleTender),
		Description:   name("Steady, low-voiced."),
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}

	// A PATCH carrying only a name must not silently reset the register: that is
	// the whole reason the draft fields are pointers.
	renamed, err := store.Update(ctx, item.ID, Draft{Name: name("Rowan Vale")})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "Rowan Vale" {
		t.Fatalf("name = %q, want the new name", renamed.Name)
	}
	if renamed.ChatVoice != configured.ChatVoice || renamed.ReactionStyle != configured.ReactionStyle {
		t.Fatalf("rename reset the axes: voice %q style %q", renamed.ChatVoice, renamed.ReactionStyle)
	}
	if renamed.Description != configured.Description {
		t.Fatalf("description = %q, want it preserved", renamed.Description)
	}
}

func TestListOrdersRecentlyUsedFirstThenUnused(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := createPersona(t, store, "Ash")
	second := createPersona(t, store, "Mara")
	createPersona(t, store, "Zephyr")

	if _, err := store.MarkUsed(ctx, second.ID); err != nil {
		t.Fatalf("mark second used: %v", err)
	}
	if _, err := store.MarkUsed(ctx, first.ID); err != nil {
		t.Fatalf("mark first used: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := make([]string, 0, len(listed))
	for _, item := range listed {
		got = append(got, item.Name)
	}
	// Never-used personas sort last so the grid leads with what the user reaches
	// for, and MarkUsed is what moves a tile rather than editing it.
	want := []string{"Ash", "Mara", "Zephyr"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestMarkUsedDoesNotCountAsAnEdit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")

	if _, err := store.MarkUsed(ctx, item.ID); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	reloaded, err := store.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reloaded.LastUsedAt == "" {
		t.Fatal("last used was not recorded")
	}
	if reloaded.UpdatedAt != item.UpdatedAt {
		t.Fatal("selecting a persona must not look like editing it")
	}
}

func TestDuplicateCopiesTheAxesAndThePortrait(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")
	if _, err := store.Update(ctx, item.ID, Draft{
		ChatVoice:     name(config.LLMChatVoiceExplicit),
		ReactionStyle: name(config.LLMReactionStyleDominant),
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	portrait := jpegOfSize(t, 64, 80)
	if _, err := store.SavePortrait(ctx, item.ID, portrait); err != nil {
		t.Fatalf("save portrait: %v", err)
	}

	copied, err := store.Duplicate(ctx, item.ID)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if copied.ID == item.ID {
		t.Fatal("duplicate reused the source ID")
	}
	if copied.Name != "Rowan copy" {
		t.Fatalf("name = %q, want a copy suffix", copied.Name)
	}
	if copied.ChatVoice != config.LLMChatVoiceExplicit || copied.ReactionStyle != config.LLMReactionStyleDominant {
		t.Fatalf("axes were not copied: voice %q style %q", copied.ChatVoice, copied.ReactionStyle)
	}
	// A duplicate that lost its picture would read as a different persona in the
	// grid, which is the opposite of what duplicating is for.
	if !copied.HasPortrait {
		t.Fatal("duplicate did not carry the portrait")
	}
	stored, err := store.readPortrait(copied.ID)
	if err != nil {
		t.Fatalf("read duplicated portrait: %v", err)
	}
	if !bytes.Equal(stored, portrait) {
		t.Fatal("duplicated portrait bytes differ from the source")
	}
	if copied.LastUsedAt != "" {
		t.Fatal("a duplicate has never been used and must not inherit that stamp")
	}
}

func TestDuplicateKeepsTheCopySuffixWithinTheNameBound(t *testing.T) {
	store := newTestStore(t)
	item := createPersona(t, store, strings.Repeat("n", MaxNameChars))

	copied, err := store.Duplicate(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if length := len([]rune(copied.Name)); length > MaxNameChars {
		t.Fatalf("duplicate name is %d runes, over the %d bound", length, MaxNameChars)
	}
	if !strings.HasSuffix(copied.Name, " copy") {
		t.Fatalf("name = %q, want a copy suffix", copied.Name)
	}
}

func TestDeleteRemovesTheRowAndThePortraitFile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")
	if _, err := store.SavePortrait(ctx, item.ID, jpegOfSize(t, 32, 40)); err != nil {
		t.Fatalf("save portrait: %v", err)
	}
	path := filepath.Join(store.PortraitDir(), item.ID+".jpg")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("portrait was not written: %v", err)
	}

	if err := store.Delete(ctx, item.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: err = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("deleting a persona left its portrait on disk")
	}
	if err := store.Delete(ctx, item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
}

func TestPortraitRejectsPayloadsTheServerWouldOtherwiseServeBack(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")

	for label, payload := range map[string][]byte{
		"empty":          {},
		"not a JPEG":     []byte("<html>not an image</html>"),
		"truncated JPEG": append([]byte{0xFF, 0xD8, 0xFF}, []byte("still not decodable")...),
		"oversize edge":  jpegOfSize(t, MaxPortraitEdge+8, 64),
	} {
		if _, err := store.SavePortrait(ctx, item.ID, payload); !errors.Is(err, ErrPortraitInvalid) {
			t.Fatalf("%s: err = %v, want ErrPortraitInvalid", label, err)
		}
	}
	if _, err := store.OpenPortrait(ctx, item.ID); !errors.Is(err, ErrPortraitNotFound) {
		t.Fatalf("open after rejections: err = %v, want ErrPortraitNotFound", err)
	}
}

// A hostile identifier must not become a path element. The store's own ID
// alphabet is the boundary, so this asserts the traversal attempts never reach
// the filesystem at all.
func TestPortraitPathsCannotEscapeTheStoreDirectory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, id := range []string{
		"../../etc/passwd",
		"persona-../../evil",
		"persona-" + strings.Repeat("f", 11),
		"persona-" + strings.Repeat("f", 13),
		"persona-ABCDEF012345",
		"persona-zzzzzzzzzzzz",
		"",
	} {
		if ValidID(id) {
			t.Fatalf("ValidID accepted %q", id)
		}
		if _, err := store.portraitPath(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("portraitPath(%q): err = %v, want ErrNotFound", id, err)
		}
		if _, err := store.SavePortrait(ctx, id, jpegOfSize(t, 16, 16)); err == nil {
			t.Fatalf("SavePortrait accepted %q", id)
		}
	}
}

func TestDeletePortraitRevertsToTheMonogramWithoutTouchingTheRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Vesper")
	if _, err := store.SavePortrait(ctx, item.ID, jpegOfSize(t, 48, 60)); err != nil {
		t.Fatalf("save portrait: %v", err)
	}

	cleared, err := store.DeletePortrait(ctx, item.ID)
	if err != nil {
		t.Fatalf("delete portrait: %v", err)
	}
	if cleared.HasPortrait || cleared.PortraitUpdatedAt != "" {
		t.Fatal("portrait was not cleared")
	}
	if cleared.Name != item.Name || cleared.ChatVoice != item.ChatVoice {
		t.Fatal("clearing a portrait changed the persona itself")
	}
	// Clearing an absent portrait is a no-op rather than an error, so a double
	// click on the control cannot produce a failure toast.
	if _, err := store.DeletePortrait(ctx, item.ID); err != nil {
		t.Fatalf("second clear: %v", err)
	}
}

func TestPortraitReplacementChangesTheCacheBuster(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")

	first, err := store.SavePortrait(ctx, item.ID, jpegOfSize(t, 32, 40))
	if err != nil {
		t.Fatalf("first portrait: %v", err)
	}
	second, err := store.SavePortrait(ctx, item.ID, jpegOfSize(t, 40, 50))
	if err != nil {
		t.Fatalf("second portrait: %v", err)
	}
	// The tile URL carries this stamp. If replacing a portrait in place did not
	// move it, the browser would keep showing the old picture.
	if first.PortraitUpdatedAt == second.PortraitUpdatedAt {
		t.Fatal("replacing a portrait did not move the cache buster")
	}
}

func TestUnknownPersonaIsNotFoundRatherThanAnError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	missing := "persona-0123456789ab"

	if _, err := store.Get(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get: err = %v, want ErrNotFound", err)
	}
	if _, err := store.Update(ctx, missing, Draft{Name: name("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update: err = %v, want ErrNotFound", err)
	}
	if _, err := store.Duplicate(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("duplicate: err = %v, want ErrNotFound", err)
	}
	if _, err := store.MarkUsed(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mark used: err = %v, want ErrNotFound", err)
	}
}
