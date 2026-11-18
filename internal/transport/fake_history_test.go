package transport

import (
	"context"
	"testing"
)

func TestBoundedFakeRetainsLatestCommandsAndLifetimeCount(t *testing.T) {
	fake := NewFake(WithHistoryLimit(2))
	for _, reason := range []string{"one", "two", "three", "four", "five"} {
		if _, err := fake.Stop(t.Context(), StopCommand{Reason: reason}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := fake.Stop(ctx, StopCommand{}); err == nil {
		t.Fatal("expected canceled dispatch")
	}
	commands, results, diag := fake.Commands(), fake.Results(), fake.Diagnostics()
	if len(commands) != 2 || commands[0].Stop.Reason != "four" || commands[1].Stop.Reason != "five" || len(results) != 2 || !results[0].OK || results[1].OK || diag.CommandCount != 5 || diag.LastCommand.ID != commands[1].ID || diag.LastCommand.Stop.Reason != "redacted" || diag.LastResult.OK {
		t.Fatalf("retained order or lifetime count is wrong: %+v", diag)
	}
	commands[0].Stop.Reason = "changed"
	if fake.Commands()[0].Stop.Reason != "four" {
		t.Fatal("history exposed an owned command")
	}
}
