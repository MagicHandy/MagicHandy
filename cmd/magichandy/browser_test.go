package main

import "testing"

func TestLaunchBrowserUsesExplicitSetupRoute(t *testing.T) {
	if got := browserLaunchRoute(true); got != "#/setup/reconfigure" {
		t.Fatalf("explicit setup route = %q, want #/setup/reconfigure", got)
	}
	if got := browserLaunchRoute(false); got != "#/chat" {
		t.Fatalf("default route = %q, want #/chat", got)
	}
}
