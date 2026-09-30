package persona

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestReplyLengthOverrideFollowsSettingsUntilSet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := createPersona(t, store, "Rowan")
	if item.ReplyLength != "" {
		t.Fatalf("a new persona overrides the reply length: %q", item.ReplyLength)
	}
	short, err := store.Update(ctx, item.ID, Draft{ReplyLength: name(" SHORT ")})
	if err != nil || short.ReplyLength != config.LLMReplyLengthShort {
		t.Fatalf("set override = %q, %v", short.ReplyLength, err)
	}
	renamed, err := store.Update(ctx, item.ID, Draft{Name: name("Rowan Vale")})
	if err != nil || renamed.ReplyLength != config.LLMReplyLengthShort {
		t.Fatalf("an unrelated edit reset the override: %q, %v", renamed.ReplyLength, err)
	}
	stored, err := store.Get(ctx, item.ID)
	if err != nil || stored.ReplyLength != config.LLMReplyLengthShort {
		t.Fatalf("stored override = %q, %v", stored.ReplyLength, err)
	}
	cleared, err := store.Update(ctx, item.ID, Draft{ReplyLength: name("")})
	if err != nil || cleared.ReplyLength != "" {
		t.Fatalf("clearing the override = %q, %v", cleared.ReplyLength, err)
	}
	if _, err := store.Update(ctx, item.ID, Draft{ReplyLength: name("epic")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown reply length err = %v, want ErrInvalid", err)
	}
}

func TestPortableArchiveCarriesAReplyLengthOverrideOnlyWhenSet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	follows := createPersona(t, store, "Follows")
	data, _, err := store.ExportArchive(ctx, follows.ID, nil)
	if err != nil {
		t.Fatalf("ExportArchive: %v", err)
	}
	if strings.Contains(string(archiveManifest(t, data)), "reply_length") {
		t.Fatal("an archive without an override names reply_length, which older builds reject")
	}

	if _, err := store.Update(ctx, follows.ID, Draft{ReplyLength: name(config.LLMReplyLengthDetailed)}); err != nil {
		t.Fatal(err)
	}
	data, _, err = store.ExportArchive(ctx, follows.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := DecodeArchive(data)
	if err != nil {
		t.Fatalf("DecodeArchive: %v", err)
	}
	imported, err := store.ImportPortable(ctx, portable, "")
	if err != nil {
		t.Fatalf("ImportPortable: %v", err)
	}
	if imported.ReplyLength != config.LLMReplyLengthDetailed {
		t.Fatalf("imported override = %q", imported.ReplyLength)
	}
}
