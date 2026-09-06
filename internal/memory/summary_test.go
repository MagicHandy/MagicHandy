package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSummaryTracksMemoryMutationsAndGlobalSwitch(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	check := func(want Summary) {
		t.Helper()
		got, err := store.Summary(context.Background())
		if err != nil || got != want {
			t.Fatalf("summary = (%+v, %v), want %+v", got, err, want)
		}
	}
	check(Summary{Enabled: true})
	first, err := store.Add("First saved fact")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add("Second saved fact"); err != nil {
		t.Fatal(err)
	}
	check(Summary{Enabled: true, Count: 2, EnabledCount: 2})
	if _, err := store.SetItemEnabled(first.ID, false); err != nil {
		t.Fatal(err)
	}
	check(Summary{Enabled: true, Count: 2, EnabledCount: 1})
	if err := store.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	check(Summary{Count: 2, EnabledCount: 1})
	if err := store.Remove(first.ID); err != nil {
		t.Fatal(err)
	}
	check(Summary{Count: 1, EnabledCount: 1})
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	check(Summary{})
}

func TestSummaryHonorsCanceledContext(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Summary(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("summary error = %v, want cancellation", err)
	}
}

func BenchmarkMemoryStateRead(b *testing.B) {
	store, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	for range maxMemories {
		if _, err := store.Add(strings.Repeat("x", maxMemoryChars)); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("FullSnapshot", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := store.Snapshot(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Summary", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := store.Summary(context.Background()); err != nil {
				b.Fatal(err)
			}
		}
	})
}
