package persona

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestPersonaCapacityRejectsTheNextRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for index := 0; index < maxPersonas; index++ {
		if _, err := store.Create(ctx, Draft{Name: name(fmt.Sprintf("Persona %03d", index))}); err != nil {
			t.Fatalf("create persona %d: %v", index, err)
		}
	}
	if _, err := store.Create(ctx, Draft{Name: name("One too many")}); !errors.Is(err, ErrLimit) {
		t.Fatalf("overflow error = %v, want ErrLimit", err)
	}
}

func TestConcurrentPersonaWritersCannotExceedCapacity(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for index := 0; index < maxPersonas-1; index++ {
		if _, err := store.Create(ctx, Draft{Name: name(fmt.Sprintf("Persona %03d", index))}); err != nil {
			t.Fatalf("fill persona %d: %v", index, err)
		}
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var writers sync.WaitGroup
	for _, displayName := range []string{"Concurrent A", "Concurrent B"} {
		writers.Add(1)
		go func(value string) {
			defer writers.Done()
			<-start
			_, err := store.Create(ctx, Draft{Name: name(value)})
			results <- err
		}(displayName)
	}
	close(start)
	writers.Wait()
	close(results)

	var created, limited int
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	if created != 1 || limited != 1 {
		t.Fatalf("concurrent results: created=%d limited=%d, want 1 and 1", created, limited)
	}
	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list personas: %v", err)
	}
	if len(items) != maxPersonas {
		t.Fatalf("persona count = %d, want %d", len(items), maxPersonas)
	}
}
