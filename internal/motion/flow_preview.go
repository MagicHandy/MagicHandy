package motion

import (
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// FlowPreview compares independent generators through the same engine plan.
type FlowPreview struct {
	Spec        FlowSpec              `json:"spec"`
	Settings    config.MotionSettings `json:"settings"`
	SettingsKey string                `json:"settings_key"`
	Candidates  []LabCandidate        `json:"candidates"`
}

// PreviewFlow returns a new-framework plan with optional historical references.
func PreviewFlow(spec FlowSpec, settings config.MotionSettings, references bool) (FlowPreview, error) {
	result := FlowPreview{Spec: spec, Settings: settings, SettingsKey: LabSettingsKey(settings)}
	if references {
		request := LabRequest{SpeedPercent: spec.SpeedPercent, CenterPercent: (spec.MinPercent + spec.MaxPercent) / 2,
			SpanPercent: spec.MaxPercent - spec.MinPercent, SpanMinPercent: max(20, spec.RangeFloorPercent),
			SpanProfile: "wander", VariationPercent: 30, RangeAnchorPercent: spec.AnchorPercent, OutboundTimePercent: 50, Seed: spec.Seed}
		if request.SpanPercent >= 20 {
			for _, method := range []string{"creative", "anchored"} {
				target, err := request.Target(method)
				if err != nil {
					return FlowPreview{}, err
				}
				plan := NewMotionPlan("reference-"+method, target, settings, 0, 0, time.Unix(0, 0))
				if err := plan.compilationError(); err != nil {
					return FlowPreview{}, err
				}
				result.Candidates = append(result.Candidates, describeLabPlan(method, plan))
			}
		}
	}
	target, err := FlowTarget(spec, settings)
	if err != nil {
		return FlowPreview{}, err
	}
	plan := NewMotionPlan("flow-preview", target, settings, 0, 0, time.Unix(0, 0))
	if err := plan.compilationError(); err != nil {
		return FlowPreview{}, err
	}
	candidate := describeLabPlan("flow", plan)
	candidate.Flow = &spec
	result.Candidates = append(result.Candidates, candidate)
	return result, nil
}
