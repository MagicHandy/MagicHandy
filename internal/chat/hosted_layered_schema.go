package chat

import "maps"

// A named geometry and manual shape edits are different composition methods.
// The hosted wire makes that choice exclusive instead of offering fields whose
// values could contradict the geometry. The manual branch retains every degree
// of freedom; named operations keep independent pace and outer-band controls.
func hostedLayeredEditSchema(fields map[string]any) map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		if required == nil {
			required = []string{}
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	manual := maps.Clone(fields)
	manual["directional_pace"] = hostedDirectionalPaceSchema()
	delete(manual, "geometry")
	manual["composition"] = map[string]any{"const": "manual"}
	controls := fields["controls"].(map[string]any)["properties"].(map[string]any)
	paceControls := map[string]any{}
	for _, name := range []string{"speed_percent", "memory_cycles", "pace_variation_percent"} {
		paceControls[name] = controls[name]
	}
	paceLayer := fields["layers"].(map[string]any)["properties"].(map[string]any)["pace"]
	geometry := map[string]any{
		"directional_pace": hostedDirectionalPaceSchema(),
		"composition":      map[string]any{"const": "geometry"},
		"controls":         object(paceControls),
		"layers":           object(map[string]any{"pace": paceLayer}),
		"evolve":           fields["evolve"],
		"geometry":         map[string]any{"const": "full_slider"},
	}
	// Decide full-slider travel before emitting controls. Sharing the same
	// discriminator with the band-preserving branch let constrained decoding
	// commit to full_slider merely by omitting that branch's nullable endpoints.
	fullFields := maps.Clone(geometry)
	fullFields["composition"] = map[string]any{"const": "full_slider"}
	delete(fullFields, "geometry")
	fullSlider := object(fullFields, "composition")
	bandControls := maps.Clone(paceControls)
	bandControls["min_percent"], bandControls["max_percent"] = controls["min_percent"], controls["max_percent"]
	geometry["controls"] = object(bandControls)
	var names []string
	for _, name := range layeredGeometries {
		if name != "full_slider" {
			names = append(names, name)
		}
	}
	geometry["geometry"] = map[string]any{"type": "string", "enum": names}
	geometry["stroke_width"] = fields["stroke_width"]
	// A phrase has no field range/center layer to follow. Existing copied pace
	// envelopes remain ordinary independent layers; new follow edits are absent.
	phrasePace := map[string]any{"oneOf": paceLayer.(map[string]any)["oneOf"].([]any)[:2]}
	phrase := object(map[string]any{"composition": map[string]any{"const": "phrase"}, "phrase": hostedPhraseSchema(), "controls": object(paceControls), "layers": object(map[string]any{"pace": phrasePace}), "evolve": fields["evolve"], "directional_pace": hostedDirectionalPaceSchema()}, "composition", "phrase")
	return map[string]any{"oneOf": []any{
		map[string]any{"const": map[string]any{}},
		object(manual, "composition"),
		object(geometry, "composition", "geometry"),
		fullSlider,
		phrase,
	}}
}

func hostedLayeredLayersSchema(layer map[string]any) map[string]any {
	properties := maps.Clone(layer["properties"].(map[string]any))
	delete(properties, "axis")
	patch := map[string]any{"type": "object", "properties": properties, "required": []string{}, "additionalProperties": false}
	// Choose edit/remove before any nullable patch field. Otherwise a decoder
	// starting with op can enter only the remove branch (edit starts at amount).
	operation := map[string]any{"oneOf": []any{
		map[string]any{"type": "object", "properties": map[string]any{"op": map[string]any{"const": "edit"}, "value": patch}, "required": []string{"op", "value"}, "additionalProperties": false},
		map[string]any{"type": "object", "properties": map[string]any{"op": map[string]any{"const": "remove"}}, "required": []string{"op"}, "additionalProperties": false},
	}}
	// Only hosted pace edits advertise the optional rate floor. Other axes and
	// the compact local contract keep their existing grammar and prompt size.
	paceProperties := maps.Clone(properties)
	paceProperties["low_rate_percent"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 100}
	paceProperties["inverted"] = map[string]any{"type": "boolean"}
	pacePatch := maps.Clone(patch)
	pacePatch["properties"] = paceProperties
	paceEdit := map[string]any{"type": "object", "properties": map[string]any{"op": map[string]any{"const": "edit"}, "value": pacePatch}, "required": []string{"op", "value"}, "additionalProperties": false}
	followFields := map[string]any{"axis": map[string]any{"type": "string", "enum": []string{"center", "range"}},
		"relation":       map[string]any{"type": "string", "enum": []string{"same", "opposite"}},
		"amount_percent": properties["amount_percent"], "low_rate_percent": paceProperties["low_rate_percent"]}
	follow := map[string]any{"type": "object", "properties": map[string]any{"op": map[string]any{"const": "follow"}, "value": map[string]any{
		"type": "object", "properties": followFields, "required": []string{"axis", "relation", "amount_percent", "low_rate_percent"}, "additionalProperties": false}},
		"required": []string{"op", "value"}, "additionalProperties": false}
	paceOperation := map[string]any{"oneOf": []any{paceEdit, operation["oneOf"].([]any)[1], follow}}
	return map[string]any{"type": "object", "properties": map[string]any{"range": operation, "center": operation, "pace": paceOperation}, "required": []string{}, "additionalProperties": false}
}
