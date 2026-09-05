package motion

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// DynamicExperiment separates two expressive axes that the production
// variation control couples. Both affect authored intent before the ordinary
// C2 fit, runtime envelope, sampler, sanitizer and transport dispatch.
type DynamicExperiment struct {
	RangeAnchorPercent  int `json:"range_anchor_percent"`
	OutboundTimePercent int `json:"outbound_time_percent"`
}

func normalizeDynamicExperiment(value *DynamicExperiment) *DynamicExperiment {
	if value == nil {
		return nil
	}
	normalized := *value
	normalized.RangeAnchorPercent = clamp(normalized.RangeAnchorPercent, 0, 100)
	normalized.OutboundTimePercent = clamp(normalized.OutboundTimePercent, 25, 75)
	if normalized.RangeAnchorPercent == 50 && normalized.OutboundTimePercent == 50 {
		return nil
	}
	return &normalized
}

func dynamicExperimentalLegMillis(duration int64, left, right float64, value *DynamicExperiment) int64 {
	if value == nil || left == right {
		return duration
	}
	share := float64(value.OutboundTimePercent) / 100
	if right < left {
		share = 1 - share
	}
	return max(int64(1), int64(math.Round(float64(duration)*2*share)))
}

// LabRequest is a bounded semantic experiment, never a timed-point API.
// Its seed permits matched, reproducible comparisons without hidden variation.
type LabRequest struct {
	SpeedPercent        int    `json:"speed_percent"`
	CenterPercent       int    `json:"center_percent"`
	SpanPercent         int    `json:"span_percent"`
	SpanMinPercent      int    `json:"span_min_percent"`
	SpanProfile         string `json:"span_profile"`
	VariationPercent    int    `json:"variation_percent"`
	RangeAnchorPercent  int    `json:"range_anchor_percent"`
	OutboundTimePercent int    `json:"outbound_time_percent"`
	Seed                uint32 `json:"seed"`
}

// LabStart binds an explicit audition to the settings used for preview.
type LabStart struct {
	Request     LabRequest `json:"request"`
	Method      string     `json:"method"`
	SettingsKey string     `json:"settings_key"`
	Flow        *FlowSpec  `json:"flow,omitempty"`
}

const (
	// TargetSourceMotionLab identifies an explicitly started experiment.
	TargetSourceMotionLab = "motion_lab"
	labPreviewMillis      = int64(12_000)
)

var labMethods = []string{"creative", "anchored", "directional", "combined"}

// LabSettingsKey detects a changed limit/calibration before audition.
func LabSettingsKey(settings config.MotionSettings) string {
	encoded, _ := json.Marshal(settings)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func (r LabRequest) validate() error {
	if r.SpeedPercent < 1 || r.SpeedPercent > 100 ||
		r.CenterPercent < 0 || r.CenterPercent > 100 ||
		r.SpanPercent < MinimumDynamicSpanPercent || r.SpanPercent > 100 ||
		r.SpanMinPercent < MinimumDynamicSpanPercent || r.SpanMinPercent > r.SpanPercent ||
		r.VariationPercent < 0 || r.VariationPercent > 100 ||
		r.RangeAnchorPercent < 0 || r.RangeAnchorPercent > 100 ||
		r.OutboundTimePercent < 25 || r.OutboundTimePercent > 75 || r.Seed == 0 {
		return errors.New("motion lab values are outside their supported ranges")
	}
	if !ValidDynamicSpanProfile(r.SpanProfile) {
		return errors.New("motion lab requires a supported span profile")
	}
	return nil
}

// Target resolves a lab method through the same Creative semantic compiler.
func (r LabRequest) Target(method string) (MotionTarget, error) {
	if err := r.validate(); err != nil {
		return MotionTarget{}, err
	}
	expression := DynamicExperiment{RangeAnchorPercent: 50, OutboundTimePercent: 50}
	switch method {
	case "creative":
	case "anchored":
		expression.RangeAnchorPercent = r.RangeAnchorPercent
	case "directional":
		expression.OutboundTimePercent = r.OutboundTimePercent
	case "combined":
		expression.RangeAnchorPercent = r.RangeAnchorPercent
		expression.OutboundTimePercent = r.OutboundTimePercent
	default:
		return MotionTarget{}, errors.New("unknown motion lab method")
	}
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: r.CenterPercent, SpanPercent: r.SpanPercent,
		SpanMinPercent: r.SpanMinPercent, SpanProfile: r.SpanProfile,
		VariationPercent: r.VariationPercent, PhraseSeed: r.Seed,
		SegmentSeconds: 12, Experiment: &expression,
	})
	return MotionTarget{Label: "Motion Lab / " + method, Source: TargetSourceMotionLab,
		SpeedPercent: r.SpeedPercent, Dynamic: &definition}, nil
}

// LabSample is a planned semantic position and a velocity estimate, not
// a wire point or measured device feedback. Transport fitting is deliberately
// excluded from the preview and still occurs normally during an audition.
type LabSample struct {
	TimeMillis        int64   `json:"time_ms"`
	PositionPercent   float64 `json:"position_percent"`
	VelocityPerSecond float64 `json:"velocity_percent_per_second"`
}

// LabCandidate contains full-loop diagnostics and a fixed 12s excerpt.
type LabCandidate struct {
	Method              string            `json:"method"`
	Target              MotionTarget      `json:"target"`
	PeriodMillis        int64             `json:"period_ms"`
	Perceptual          PerceptualSummary `json:"perceptual"`
	OutboundTimePercent float64           `json:"outbound_time_percent"`
	MaximumAcceleration float64           `json:"maximum_acceleration"`
	MaximumJerk         float64           `json:"maximum_jerk"`
	Samples             []LabSample       `json:"samples"`
	Flow                *FlowSpec         `json:"flow,omitempty"`
}

// LabPreview is fully computed on the backend without creating an
// Engine, acquiring a controller, changing settings, or contacting a device.
type LabPreview struct {
	Version       int                   `json:"version"`
	Request       LabRequest            `json:"request"`
	Settings      config.MotionSettings `json:"settings"`
	SettingsKey   string                `json:"settings_key"`
	PreviewMillis int64                 `json:"preview_ms"`
	Candidates    []LabCandidate        `json:"candidates"`
}

// PreviewMotionLab compares identical source intent, seed and saved limits.
func PreviewMotionLab(request LabRequest, settings config.MotionSettings) (LabPreview, error) {
	result := LabPreview{Version: 1, Request: request, Settings: settings,
		SettingsKey: LabSettingsKey(settings), PreviewMillis: labPreviewMillis}
	for _, method := range labMethods {
		target, err := request.Target(method)
		if err != nil {
			return LabPreview{}, err
		}
		plan := NewMotionPlan("lab-"+method, target, settings, 0, 0, time.Unix(0, 0))
		if err := plan.compilationError(); err != nil {
			return LabPreview{}, err
		}
		result.Candidates = append(result.Candidates, describeLabPlan(method, plan))
	}
	return result, nil
}

func describeLabPlan(method string, plan MotionPlan) LabCandidate {
	factor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
	result := LabCandidate{Method: method, Target: plan.Target, PeriodMillis: plan.PeriodMillis,
		Perceptual:          plan.Perceptual,
		MaximumAcceleration: plan.curve.maximumAccelerationPerMillis2() * 1e6 / (factor * factor),
		MaximumJerk:         plan.curve.maximumJerkPerMillis3() * 1e9 / (factor * factor * factor),
	}
	outbound := int64(0)
	for index := 1; index < len(plan.curve.authoredKnots); index++ {
		left, right := plan.curve.authoredKnots[index-1], plan.curve.authoredKnots[index]
		if right.PositionPercent > left.PositionPercent {
			outbound += right.TimeMillis - left.TimeMillis
		}
	}
	result.OutboundTimePercent = float64(outbound) * 100 / float64(plan.curve.duration)
	// Include exact turning points in addition to regular visual probes so a
	// short reversal is not hidden merely by the chart's sampling interval.
	times := plan.knotTimesBetween(0, labPreviewMillis)
	for at := int64(0); at <= labPreviewMillis; at += 20 {
		times = append(times, at)
	}
	slices.Sort(times)
	for _, at := range uniqueMillis(times) {
		result.Samples = append(result.Samples, LabSample{
			TimeMillis: at, PositionPercent: plan.SampleAt(at).PositionPercent,
			VelocityPerSecond: plan.VelocityAt(at),
		})
	}
	return result
}
