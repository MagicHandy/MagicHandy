package httpapi

import (
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) expandLabTestPreset(body *labTestCreateRequest) error {
	if body.Preset == "" {
		return nil
	}
	if len(body.Steps) != 0 {
		return errors.New("choose a preset or custom test steps")
	}
	state := s.labState()
	target := observationRequest{Source: "motion", Method: "flow", Spec: &state.Current, SettingsKey: state.SettingsKey}
	if body.Target != nil {
		target = *body.Target
	}
	switch body.Preset {
	case "motion_experiments":
		if target.Source != "motion" || target.Spec == nil {
			return errors.New("choose a motion preview for the comparison")
		}
		if body.Title == "" {
			body.Title = "Motion experiments"
		}
		for _, experiment := range motion.FlowExperiments(*target.Spec) {
			stepTarget := target
			stepTarget.Method, stepTarget.Spec = "flow", &experiment.Spec
			body.Steps = append(body.Steps, labTestStepRequest{Title: experiment.Name, Instruction: experiment.Description, Target: stepTarget})
		}
	case "motion_comparison":
		if target.Source != "motion" || target.Spec == nil {
			return errors.New("choose a motion preview for the comparison")
		}
		if body.Title == "" {
			body.Title = "Motion feel check"
		}
		methods := []string{"creative", "anchored", "flow"}
		if target.Spec.MaxPercent-target.Spec.MinPercent < 20 {
			methods = []string{"flow"}
		}
		for _, method := range methods {
			stepTarget := target
			stepTarget.Method = method
			title := map[string]string{"creative": "Creative baseline", "anchored": "Anchored range", "flow": "Continuous flow"}[method]
			body.Steps = append(body.Steps, labTestStepRequest{Title: title, Instruction: "Look for smooth reversals, gradual range changes and a comfortable pace. Rate the result and describe any jerks, stalls or repetition.", Target: stepTarget})
		}
	case "llm_comparison":
		if target.Source != "llm" {
			return errors.New("choose an LLM reply for the comparison")
		}
		if body.Title == "" {
			body.Title = "LLM request check"
		}
		body.Steps = []labTestStepRequest{
			{Title: "Before the request", Instruction: "Review the starting motion so you have a reference for the requested change.", Target: target, Phase: "before"},
			{Title: "After the request", Instruction: "Compare the reply and motion with the request. Did it change the right thing while preserving everything else?", Target: target, Phase: "after"},
		}
	default:
		return errors.New("unknown test sequence preset")
	}
	return nil
}
