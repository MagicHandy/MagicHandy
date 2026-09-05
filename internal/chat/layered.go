package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// LayeredEdit describes changes to a persistent continuous score. It contains
// no device commands, playback clock, ordered sections or turnaround shaping.
type LayeredEdit struct {
	Geometry     string                     `json:"geometry,omitempty"`
	StrokeWidth  *LayeredStrokeWidth        `json:"stroke_width,omitempty"`
	Controls     map[string]json.RawMessage `json:"controls,omitempty"`
	ChangeBy     map[string]json.RawMessage `json:"change_by,omitempty"`
	Layers       []LayeredLayerEdit         `json:"layers,omitempty"`
	RemoveLayers []string                   `json:"remove_layers,omitempty"`
	Evolve       bool                       `json:"evolve,omitempty"`
}

// LayeredStrokeWidth pairs the shortest and widest requested stroke.
type LayeredStrokeWidth struct {
	MinPercent int `json:"min_percent"`
	MaxPercent int `json:"max_percent"`
}

// LayeredLayerEdit updates only supplied attributes of one modulation axis.
type LayeredLayerEdit struct {
	Axis               string  `json:"axis"`
	AmountPercent      *int    `json:"amount_percent,omitempty"`
	PeriodCycles       *int    `json:"period_cycles,omitempty"`
	PeriodChangeCycles *int    `json:"period_change_cycles,omitempty"`
	PhasePercent       *int    `json:"phase_percent,omitempty"`
	Shape              *string `json:"shape,omitempty"`
}

var layeredControlNames = []string{"min_percent", "max_percent", "speed_percent", "anchor_percent", "memory_cycles", "pace_variation_percent", "variation_mode"}

// DefaultLayeredScore varies reach, location and pace on independent time scales.
func DefaultLayeredScore(speed int) motion.FlowSpec {
	spec := motion.DefaultFlowSpec()
	spec.SpeedPercent, spec.AnchorPercent, spec.RangeFloorPercent = speed, 50, 20
	spec.VariationMode, spec.MemoryCycles, spec.PaceVariationPercent = "drift", 10, 12
	spec.Layers = []motion.FlowLayer{
		{Axis: "center", AmountPercent: 35, PeriodCycles: 14, PhasePercent: 17, Shape: "drift"},
		{Axis: "pace", AmountPercent: 20, PeriodCycles: 22, PhasePercent: 53, Shape: "drift"},
	}
	return spec
}

// ParseLayeredReply is the shared Lab/production contract. It keeps the raw
// rejected proposal intact for callers; no inferred repair or preset is applied.
func ParseLayeredReply(raw string, current motion.FlowSpec, limits config.MotionSettings) (AssistantResponse, motion.FlowSpec, []string, error) {
	if err := rejectLayeredNulls(raw); err != nil {
		return AssistantResponse{}, current, nil, err
	}
	var proposal struct {
		Reply   string       `json:"reply"`
		NewMood *Mood        `json:"new_mood,omitempty"`
		Edits   *LayeredEdit `json:"edits"`
	}
	if err := decodeLabObject(raw, &proposal); err != nil {
		return AssistantResponse{}, current, nil, err
	}
	if proposal.Edits == nil {
		return AssistantResponse{}, current, nil, errors.New("edits is required; use an empty object for no change")
	}
	if strings.TrimSpace(proposal.Reply) == "" || len(proposal.Reply) > 12000 {
		return AssistantResponse{}, current, nil, errors.New("a non-empty bounded reply is required")
	}
	if proposal.NewMood != nil {
		if _, ok := validMood(*proposal.NewMood); !ok {
			return AssistantResponse{}, current, nil, errors.New("unknown mood")
		}
	}
	next, err := ApplyLayeredEdit(*proposal.Edits, current, limits)
	return AssistantResponse{Reply: proposal.Reply, NewMood: proposal.NewMood}, next, labChangedControls(current, next), err
}

func rejectLayeredNulls(raw string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return err
	}
	for name, value := range fields {
		if string(value) == "null" {
			return fmt.Errorf("%s must be omitted instead of null", name)
		}
	}
	if value, ok := fields["edits"]; ok {
		if err := json.Unmarshal(value, &fields); err != nil {
			return err
		}
		for name, value := range fields {
			if string(value) == "null" {
				return fmt.Errorf("%s must be omitted instead of null", name)
			}
		}
	}
	var layers []map[string]json.RawMessage
	if value, ok := fields["layers"]; ok {
		if err := json.Unmarshal(value, &layers); err != nil {
			return err
		}
		for _, layer := range layers {
			if len(layer) < 2 {
				return errors.New("each layer edit needs an axis and at least one changed attribute")
			}
			for name, value := range layer {
				if string(value) == "null" {
					return fmt.Errorf("layer %s must be omitted instead of null", name)
				}
			}
		}
	}
	return nil
}

// ApplyLayeredEdit merges an edit transactionally. A rejected edit never mutates
// the original score, including its layer backing array.
func ApplyLayeredEdit(edit LayeredEdit, current motion.FlowSpec, limits config.MotionSettings) (motion.FlowSpec, error) {
	if len(current.Steps) != 0 {
		return current, errors.New("layered mode needs a continuous score without ordered sections; start a new Layered score")
	}
	for _, fields := range []map[string]json.RawMessage{edit.Controls, edit.ChangeBy} {
		for name := range fields {
			if !slices.Contains(layeredControlNames, name) {
				return current, fmt.Errorf("unsupported Layered control: %s", name)
			}
		}
	}
	next := *motion.CloneFlowSpec(&current)
	encoded, _ := json.Marshal(next)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	if err := mergeLabControls(fields, edit.Controls); err != nil {
		return current, err
	}
	if err := mergeLabAdjustments(fields, edit.Controls, edit.ChangeBy, current); err != nil {
		return current, err
	}
	encoded, _ = json.Marshal(fields)
	_ = json.Unmarshal(encoded, &next)
	if err := applyLayeredGeometryEdit(&next, edit); err != nil {
		return current, err
	}
	if edit.StrokeWidth != nil {
		if edit.StrokeWidth.MinPercent < 10 || edit.StrokeWidth.MaxPercent < edit.StrokeWidth.MinPercent {
			return current, errors.New("stroke_width requires both min_percent and max_percent, with 10 <= min <= max")
		}
		next.RangeFloorPercent, next.RangeCeilingPercent = edit.StrokeWidth.MinPercent, edit.StrokeWidth.MaxPercent
	}
	if err := applyLayeredLayers(&next, edit.Layers, edit.RemoveLayers); err != nil {
		return current, err
	}
	if err := validateLayeredGeometry(next, edit.Geometry); err != nil {
		return current, err
	}
	if edit.Evolve {
		next.Seed = freshLayeredSeed(current.Seed)
	}
	if err := next.Validate(limits); err != nil {
		return current, err
	}
	return next, nil
}

func applyLayeredLayers(next *motion.FlowSpec, edits []LayeredLayerEdit, remove []string) error {
	if len(edits) > 3 || len(remove) > 3 {
		return errors.New("layered mode supports at most three independent axes")
	}
	seen := map[string]bool{}
	for _, axis := range remove {
		if !layeredAxis(axis) || seen[axis] {
			return errors.New("remove_layers requires distinct range, center or pace axes")
		}
		seen[axis] = true
	}
	next.Layers = slices.DeleteFunc(next.Layers, func(layer motion.FlowLayer) bool { return seen[layer.Axis] })
	for _, edit := range edits {
		if edit.PeriodCycles != nil && edit.PeriodChangeCycles != nil {
			return errors.New("set or adjust a layer period, not both")
		}
		if !layeredAxis(edit.Axis) || seen[edit.Axis] {
			return errors.New("layer edits need distinct axes and cannot update an axis being removed")
		}
		seen[edit.Axis] = true
		index := slices.IndexFunc(next.Layers, func(layer motion.FlowLayer) bool { return layer.Axis == edit.Axis })
		if index < 0 {
			if edit.PeriodChangeCycles != nil {
				return errors.New("relative layer timing needs an existing axis")
			}
			next.Layers = append(next.Layers, motion.FlowLayer{Axis: edit.Axis, AmountPercent: 30, PeriodCycles: 12, Shape: "drift"})
			index = len(next.Layers) - 1
		}
		applyLayeredLayer(&next.Layers[index], edit)
	}
	return nil
}

func layeredAxis(axis string) bool { return axis == "range" || axis == "center" || axis == "pace" }

func applyLayeredLayer(layer *motion.FlowLayer, edit LayeredLayerEdit) {
	if edit.AmountPercent != nil {
		layer.AmountPercent = *edit.AmountPercent
	}
	if edit.PeriodCycles != nil {
		layer.PeriodCycles = *edit.PeriodCycles
	}
	if edit.PeriodChangeCycles != nil {
		layer.PeriodCycles += *edit.PeriodChangeCycles
	}
	if edit.PhasePercent != nil {
		layer.PhasePercent = *edit.PhasePercent
	}
	if edit.Shape != nil {
		layer.Shape = *edit.Shape
	}
}
