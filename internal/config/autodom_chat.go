package config

import (
	"errors"
	"fmt"
	"time"
)

const (
	// DefaultSegmentDurationMinSec is the default minimum auto segment length.
	DefaultSegmentDurationMinSec = 45
	// DefaultSegmentDurationMaxSec is the default maximum auto segment length.
	DefaultSegmentDurationMaxSec = 60
	// DefaultPrefetchLeadSeconds is how early the next segment is planned.
	DefaultPrefetchLeadSeconds = 15

	minSegmentDurationSec = 30
	maxSegmentDurationSec = 120
	minPrefetchLeadSec    = 5
	maxPrefetchLeadSec    = 30
)

// SegmentDurationBounds returns validated segment duration limits for chat auto.
func (a AutoDomSettings) SegmentDurationBounds() (minSec, maxSec int) {
	minSec = a.SegmentDurationMinSec
	maxSec = a.SegmentDurationMaxSec
	if minSec == 0 {
		minSec = DefaultSegmentDurationMinSec
	}
	if maxSec == 0 {
		maxSec = DefaultSegmentDurationMaxSec
	}
	if minSec > maxSec {
		maxSec = minSec
	}
	return minSec, maxSec
}

// PrefetchLead returns how long before segment end prefetch should start.
func (a AutoDomSettings) PrefetchLead() time.Duration {
	seconds := a.PrefetchLeadSeconds
	if seconds == 0 {
		seconds = DefaultPrefetchLeadSeconds
	}
	return time.Duration(seconds) * time.Second
}

// ShouldWaitForUserMessage reports whether chat auto waits for the first user message.
func (a AutoDomSettings) ShouldWaitForUserMessage() bool {
	if a.WaitForUserMessage == nil {
		return true
	}
	return *a.WaitForUserMessage
}

func boolPtr(v bool) *bool {
	return &v
}

func normalizeAutoDomChatFields(settings AutoDomSettings) AutoDomSettings {
	if settings.SegmentDurationMinSec == 0 {
		settings.SegmentDurationMinSec = DefaultSegmentDurationMinSec
	}
	if settings.SegmentDurationMaxSec == 0 {
		settings.SegmentDurationMaxSec = DefaultSegmentDurationMaxSec
	}
	if settings.SegmentDurationMinSec > settings.SegmentDurationMaxSec {
		settings.SegmentDurationMaxSec = settings.SegmentDurationMinSec
	}
	if settings.PrefetchLeadSeconds == 0 {
		settings.PrefetchLeadSeconds = DefaultPrefetchLeadSeconds
	}
	return settings
}

func validateAutoDomChatFields(settings AutoDomSettings) error {
	if settings.SegmentDurationMinSec < minSegmentDurationSec || settings.SegmentDurationMinSec > maxSegmentDurationSec {
		return fmt.Errorf("segment duration minimum must be between %d and %d seconds", minSegmentDurationSec, maxSegmentDurationSec)
	}
	if settings.SegmentDurationMaxSec < minSegmentDurationSec || settings.SegmentDurationMaxSec > maxSegmentDurationSec {
		return fmt.Errorf("segment duration maximum must be between %d and %d seconds", minSegmentDurationSec, maxSegmentDurationSec)
	}
	if settings.SegmentDurationMinSec > settings.SegmentDurationMaxSec {
		return errors.New("segment duration minimum cannot exceed maximum")
	}
	if settings.PrefetchLeadSeconds < minPrefetchLeadSec || settings.PrefetchLeadSeconds > maxPrefetchLeadSec {
		return fmt.Errorf("prefetch lead must be between %d and %d seconds", minPrefetchLeadSec, maxPrefetchLeadSec)
	}
	return nil
}
