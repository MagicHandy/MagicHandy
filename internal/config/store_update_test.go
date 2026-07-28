package config

import (
	"sync"
	"testing"
)

func TestStoreUpdateSerializesConcurrentFieldMutations(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, updateErr := store.Update(func(current Settings) (Settings, error) {
			close(firstEntered)
			<-releaseFirst
			current.UI.Locale = LocaleJapanese
			return current, nil
		})
		errs <- updateErr
	}()

	<-firstEntered
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, updateErr := store.Update(func(current Settings) (Settings, error) {
			current.Voice.SpeakReplies = true
			return current, nil
		})
		errs <- updateErr
	}()

	close(releaseFirst)
	wg.Wait()
	close(errs)
	for updateErr := range errs {
		if updateErr != nil {
			t.Fatalf("Update: %v", updateErr)
		}
	}

	got, _ := store.Snapshot()
	if got.UI.Locale != LocaleJapanese || !got.Voice.SpeakReplies {
		t.Fatalf("concurrent field updates were not both retained: locale=%q speak_replies=%v", got.UI.Locale, got.Voice.SpeakReplies)
	}

	reloaded, err := OpenStore(store.DataDir())
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() {
		if err := reloaded.Close(); err != nil {
			t.Errorf("close reloaded store: %v", err)
		}
	})
	got, _ = reloaded.Snapshot()
	if got.UI.Locale != LocaleJapanese || !got.Voice.SpeakReplies {
		t.Fatalf("durable settings lost a concurrent field update: locale=%q speak_replies=%v", got.UI.Locale, got.Voice.SpeakReplies)
	}
}

func TestStoreUpdateRejectsNilMutation(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, _, err := store.Update(nil); err == nil {
		t.Fatal("Update(nil) succeeded")
	}
}
