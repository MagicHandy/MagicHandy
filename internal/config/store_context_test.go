package config

import (
	"context"
	"errors"
	"testing"
)

func TestCanceledUpdateCannotPublishOrPersistPreparedSettings(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	before, _ := store.Snapshot()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, _, err = store.UpdateContext(ctx, func(settings Settings) (Settings, error) {
		settings.Motion.SpeedMaxPercent = 38
		cancel()
		return settings, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled update = %v", err)
	}
	after, _ := store.Snapshot()
	if after.Motion != before.Motion {
		t.Fatal("canceled transaction was published in memory")
	}
	reopened, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	durable, _ := reopened.Snapshot()
	if durable.Motion != before.Motion {
		t.Fatal("canceled transaction changed durable settings")
	}
}
