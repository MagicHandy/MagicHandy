package config

import "testing"

func TestAutoDomSegmentDurationBounds(t *testing.T) {
	settings := AutoDomSettings{
		SegmentDurationMinSec: 50,
		SegmentDurationMaxSec: 40,
	}
	minSec, maxSec := settings.SegmentDurationBounds()
	if minSec != 50 || maxSec != 50 {
		t.Fatalf("bounds = %d–%d, want 50–50 when min > max", minSec, maxSec)
	}
}

func TestValidateAutoDomChatFields(t *testing.T) {
	settings := DefaultSettings().AutoDom
	settings.SegmentDurationMinSec = 20
	if err := validateAutoDomChatFields(settings); err == nil {
		t.Fatal("expected error for segment min below limit")
	}
}
