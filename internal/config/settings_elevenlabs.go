package config

import (
	"errors"
	"math"
	"strings"
)

// ElevenLabsDelivery overrides the selected ElevenLabs voice's own settings
// for every reply. A nil value keeps the voice's setting. The worker sends
// only the settings the selected model honors.
type ElevenLabsDelivery struct {
	Stability  *float64 `json:"stability,omitempty"`
	Similarity *float64 `json:"similarity,omitempty"`
	Style      *float64 `json:"style,omitempty"`
	Speed      *float64 `json:"speed,omitempty"`
	// Language is an ISO 639 code for models that accept one; empty detects
	// the language automatically.
	Language string `json:"language,omitempty"`
}

func (d ElevenLabsDelivery) clone() ElevenLabsDelivery {
	copyFloat := func(value *float64) *float64 {
		if value == nil {
			return nil
		}
		copied := *value
		return &copied
	}
	return ElevenLabsDelivery{
		Stability:  copyFloat(d.Stability),
		Similarity: copyFloat(d.Similarity),
		Style:      copyFloat(d.Style),
		Speed:      copyFloat(d.Speed),
		Language:   d.Language,
	}
}

func normalizeElevenLabsDelivery(delivery ElevenLabsDelivery) ElevenLabsDelivery {
	delivery = delivery.clone()
	delivery.Language = strings.ToLower(strings.TrimSpace(delivery.Language))
	return delivery
}

func validateElevenLabsDelivery(delivery ElevenLabsDelivery) error {
	inRange := func(value *float64, low, high float64) bool {
		return value == nil || (!math.IsNaN(*value) && *value >= low && *value <= high)
	}
	if !inRange(delivery.Stability, 0, 1) || !inRange(delivery.Similarity, 0, 1) || !inRange(delivery.Style, 0, 1) {
		return errors.New("ElevenLabs stability, similarity and style must be between 0 and 1")
	}
	if !inRange(delivery.Speed, 0.7, 1.2) {
		return errors.New("ElevenLabs speed must be between 0.7 and 1.2")
	}
	if language := delivery.Language; language != "" {
		if len(language) < 2 || len(language) > 3 || strings.Trim(language, "abcdefghijklmnopqrstuvwxyz") != "" {
			return errors.New("ElevenLabs language must be a two- or three-letter ISO 639 code")
		}
	}
	return nil
}
