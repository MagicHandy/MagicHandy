package persona

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestPortableArchiveRoundTripPreservesOwnedDataUnderFreshIDs(t *testing.T) {
	ctx := context.Background()
	source := newTestStore(t)
	item, portrait, profile := portableSourceFixture(t, source)

	first, _, err := source.ExportArchive(ctx, item.ID, profile)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	second, _, err := source.ExportArchive(ctx, item.ID, profile)
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("unchanged persona exports are not deterministic")
	}
	requireNoRuntimeMetadata(t, archiveManifest(t, first), item)

	portable, err := DecodeArchive(first)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	requirePortableProfile(t, portable, profile)

	target := newTestStore(t)
	imported, err := target.ImportPortable(ctx, portable, "user-local-profile")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	requireImportedPersona(t, imported, item)
	importedPortrait, err := target.readPortrait(imported.ID)
	if err != nil {
		t.Fatalf("read imported portrait: %v", err)
	}
	if !bytes.Equal(importedPortrait, portrait) {
		t.Fatal("imported portrait differs from source")
	}
	lore, err := target.ListLore(ctx, imported.ID)
	if err != nil {
		t.Fatalf("list imported lore: %v", err)
	}
	if len(lore) != 1 ||
		lore[0].PersonaID != imported.ID ||
		strings.Join(lore[0].Keywords, ",") != "velvet,slow burn" {
		t.Fatalf("imported lore = %#v", lore)
	}
}

func portableSourceFixture(t *testing.T, store *Store) (Persona, []byte, *chat.PromptSet) {
	t.Helper()
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")
	item, err := store.Update(ctx, item.ID, Draft{
		Description:      name("Steady and low-voiced."),
		ChatVoice:        name(config.LLMChatVoiceIntimate),
		ReactionStyle:    name(config.LLMReactionStyleTender),
		PromptSetID:      name("user-source-profile"),
		DefaultFocusArea: name(chat.AreaZoneBase),
		LoreMode:         name(LoreModeRelevant),
	})
	if err != nil {
		t.Fatalf("configure source: %v", err)
	}
	if _, err := store.CreateLore(ctx, item.ID, LoreDraft{
		Text:     name("Blue velvet is familiar."),
		Keywords: &[]string{"Velvet", "slow burn"},
	}); err != nil {
		t.Fatalf("create source lore: %v", err)
	}
	portrait := jpegOfSize(t, 96, 128)
	if _, err := store.SavePortrait(ctx, item.ID, portrait); err != nil {
		t.Fatalf("save source portrait: %v", err)
	}
	return item, portrait, &chat.PromptSet{
		ID:     "user-source-profile",
		Name:   "Rowan behavior",
		System: "Keep the conversation measured and attentive.",
	}
}

func requireNoRuntimeMetadata(t *testing.T, manifest []byte, item Persona) {
	t.Helper()
	for _, runtimeValue := range []string{item.ID, item.CreatedAt, "last_used_at", "portrait_updated_at"} {
		if bytes.Contains(manifest, []byte(runtimeValue)) {
			t.Fatalf("portable manifest leaked runtime value %q", runtimeValue)
		}
	}
}

func requirePortableProfile(t *testing.T, portable PortableArchive, profile *chat.PromptSet) {
	t.Helper()
	if portable.BehaviorProfile == nil {
		t.Fatal("portable behavior profile is missing")
	}
	if portable.BehaviorProfile.Name != profile.Name ||
		portable.BehaviorProfile.System != profile.System {
		t.Fatalf("behavior profile = %#v", portable.BehaviorProfile)
	}
}

func requireImportedPersona(t *testing.T, imported, source Persona) {
	t.Helper()
	if imported.ID == source.ID || imported.PromptSetID != "user-local-profile" {
		t.Fatalf("imported identity/profile = %q / %q", imported.ID, imported.PromptSetID)
	}
	if imported.Name != source.Name ||
		imported.Description != source.Description ||
		imported.ChatVoice != source.ChatVoice ||
		imported.ReactionStyle != source.ReactionStyle ||
		imported.DefaultFocusArea != source.DefaultFocusArea ||
		imported.LoreMode != source.LoreMode {
		t.Fatalf("imported persona lost fields: %#v", imported)
	}
	if !imported.HasPortrait || imported.LoreCount != 1 || imported.LastUsedAt != "" {
		t.Fatalf("imported derived state = %#v", imported)
	}
}

func TestBuiltinBehaviorProfileExportsOnlyItsStableReference(t *testing.T) {
	store := newTestStore(t)
	item := createPersona(t, store, "Rowan")
	item, err := store.Update(context.Background(), item.ID, Draft{
		PromptSetID: name(chat.DefaultPromptSetID),
	})
	if err != nil {
		t.Fatalf("select built-in profile: %v", err)
	}
	profile, ok := chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	if !ok {
		t.Fatal("default profile is not built in")
	}
	archive, _, err := store.ExportArchive(context.Background(), item.ID, &profile)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	portable, err := DecodeArchive(archive)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if portable.BehaviorProfile == nil ||
		!portable.BehaviorProfile.Builtin ||
		portable.BehaviorProfile.ID != chat.DefaultPromptSetID {
		t.Fatalf("built-in profile = %#v", portable.BehaviorProfile)
	}
	if portable.BehaviorProfile.Name != "" || portable.BehaviorProfile.System != "" {
		t.Fatal("built-in implementation text was copied into the portable archive")
	}
}

func TestDecodeArchiveRejectsUnexpectedOrOversizedEntries(t *testing.T) {
	valid := validPortableArchive()
	manifest, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	for label, archive := range map[string][]byte{
		"path traversal": zipFixture(t, map[string][]byte{
			archiveManifestName: manifest,
			"../portrait.jpg":   []byte("not allowed"),
		}),
		"unknown entry": zipFixture(t, map[string][]byte{
			archiveManifestName: manifest,
			"notes.txt":         []byte("not allowed"),
		}),
		"manifest bomb": zipFixture(t, map[string][]byte{
			archiveManifestName: bytes.Repeat([]byte("x"), maxArchiveManifestBytes+1),
		}),
	} {
		if _, err := DecodeArchive(archive); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: err = %v, want ErrInvalid", label, err)
		}
	}
}

func TestDecodeArchiveRejectsUnknownJSONAndAssetMismatches(t *testing.T) {
	unknownField := []byte(`{
		"schema":"magichandy.persona",
		"version":1,
		"persona":{
			"name":"Rowan",
			"description":"",
			"chat_voice":"warm",
			"reaction_style":"neutral",
			"prompt_set_id":"",
			"default_focus_area":"full",
			"lore_mode":"off",
			"capability_gates":{"motion":true}
		},
		"lore":[]
	}`)
	undeclaredPortrait := validPortableArchive()
	undeclaredManifest, err := json.Marshal(undeclaredPortrait)
	if err != nil {
		t.Fatalf("encode undeclared fixture: %v", err)
	}

	for label, archive := range map[string][]byte{
		"unknown JSON": zipFixture(t, map[string][]byte{
			archiveManifestName: unknownField,
		}),
		"undeclared portrait": zipFixture(t, map[string][]byte{
			archiveManifestName: undeclaredManifest,
			archivePortraitName: jpegOfSize(t, 32, 40),
		}),
	} {
		if _, err := DecodeArchive(archive); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: err = %v, want ErrInvalid", label, err)
		}
	}
}

func validPortableArchive() PortableArchive {
	return PortableArchive{
		Schema:  archiveSchema,
		Version: archiveVersion,
		Persona: PortablePersona{
			Name:             "Rowan",
			ChatVoice:        config.LLMChatVoiceWarm,
			ReactionStyle:    config.LLMReactionStyleNeutral,
			DefaultFocusArea: chat.AreaZoneFull,
			LoreMode:         LoreModeOff,
		},
		Lore: []PortableLoreEntry{},
	}
}

func archiveManifest(t *testing.T, data []byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != archiveManifestName {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatalf("open manifest: %v", err)
		}
		content, err := io.ReadAll(opened)
		_ = opened.Close()
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		return content
	}
	t.Fatal("archive has no manifest")
	return nil
}

func zipFixture(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return buffer.Bytes()
}
