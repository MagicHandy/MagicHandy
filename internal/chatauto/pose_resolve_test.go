package chatauto

import "testing"

func TestResolvePoseBlocksDowngrade(t *testing.T) {
	if got := ResolvePose(PoseOral, PoseHandjob); got != PoseOral {
		t.Fatalf("ResolvePose = %q, want oral", got)
	}
	if got := ResolvePose(PoseHandjob, PoseOral); got != PoseOral {
		t.Fatalf("ResolvePose = %q, want oral upgrade", got)
	}
}
