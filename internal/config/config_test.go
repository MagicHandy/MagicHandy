package config

import "testing"

func TestDefaultUsesLoopbackAddress(t *testing.T) {
	cfg := Default()
	if cfg.Server.Address != defaultAddress {
		t.Fatalf("default address = %q, want %q", cfg.Server.Address, defaultAddress)
	}
}
