package voice

import (
	"testing"

	"go.uber.org/goleak"
)

// The voice package spawns reader, waiter, and dispatcher goroutines per
// worker; the same lifecycle gate as motion applies — nothing may outlive
// its session.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
