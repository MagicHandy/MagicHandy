package persona

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGreetingRoundTrip(t *testing.T) {
	store := newTestStore(t)
	greeting := "*She looks up.*\n\nOh, it's you."
	item, err := store.Create(context.Background(), Draft{
		Name:     name("Annabelle"),
		Greeting: name(greeting),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Greeting != greeting {
		t.Fatalf("greeting = %q, want newlines preserved", item.Greeting)
	}
	loaded, err := store.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.Greeting != greeting {
		t.Fatalf("loaded greeting = %q", loaded.Greeting)
	}
}

func TestGreetingOmittedOnUpdatePreserved(t *testing.T) {
	store := newTestStore(t)
	item, err := store.Create(context.Background(), Draft{
		Name:     name("Lily"),
		Greeting: name("Hallo!"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := store.Update(context.Background(), item.ID, Draft{Name: name("Lily Renamed")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Greeting != "Hallo!" {
		t.Fatalf("greeting = %q, want preserved on omitted field", updated.Greeting)
	}
}

func TestGreetingOverLimitRejected(t *testing.T) {
	store := newTestStore(t)
	long := strings.Repeat("a", MaxGreetingChars+1)
	_, err := store.Create(context.Background(), Draft{Name: name("X"), Greeting: name(long)})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestDuplicateCarriesGreeting(t *testing.T) {
	store := newTestStore(t)
	item, err := store.Create(context.Background(), Draft{
		Name:     name("Original"),
		Greeting: name("Welcome back."),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	copy, err := store.Duplicate(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if copy.Greeting != "Welcome back." {
		t.Fatalf("duplicate greeting = %q", copy.Greeting)
	}
}
