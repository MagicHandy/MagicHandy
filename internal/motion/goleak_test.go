package motion

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestMotionPackageScaffold(t *testing.T) {
	t.Parallel()
}
