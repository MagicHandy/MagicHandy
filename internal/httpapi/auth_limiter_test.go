package httpapi

import (
	"testing"
	"time"
)

func TestLoginLimiterChecksAddressAndUsernameIndependently(t *testing.T) {
	limiter := newLoginLimiter()
	clock := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return clock }
	limiter.ipBurst = 3
	limiter.userBurst = 2
	limiter.ipRate = 1
	limiter.userRate = 1

	for attempt := 0; attempt < 2; attempt++ {
		if !limiter.Allow("192.0.2.1", "owner") {
			t.Fatalf("initial login attempt %d was unexpectedly throttled", attempt+1)
		}
	}
	if limiter.Allow("192.0.2.2", "owner") {
		t.Fatal("a second address bypassed the per-username bucket")
	}
	if !limiter.Allow("192.0.2.1", "other") {
		t.Fatal("independent username should have used the remaining address token")
	}
	if limiter.Allow("192.0.2.1", "third") {
		t.Fatal("new username bypassed the per-address bucket")
	}
	clock = clock.Add(2 * time.Second)
	if !limiter.Allow("192.0.2.1", "owner") {
		t.Fatal("refilled buckets did not permit a later attempt")
	}
}
