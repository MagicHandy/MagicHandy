package funscript

const (
	minActions     = 2
	maxActions     = 500_000
	maxTimestampMS = 86_400_000
	minPos         = 0
	maxPos         = 100
)

// ValidateActions validates and coerces a list of raw action maps.
func ValidateActions(actions []map[string]any) ([]Action, error) {
	if len(actions) < minActions {
		return nil, validationErrorf("funscript must contain at least %d actions (got %d).", minActions, len(actions))
	}
	if len(actions) > maxActions {
		return nil, validationErrorf("funscript exceeds maximum action count (%d).", maxActions)
	}

	validated := make([]Action, 0, len(actions))
	for index, action := range actions {
		item, err := validateAction(action, index)
		if err != nil {
			return nil, err
		}
		validated = append(validated, item)
	}

	duration := validated[len(validated)-1].At - validated[0].At
	if duration <= 0 {
		return nil, validationErrorf("funscript actions must span a positive duration (last 'at' must exceed first 'at').")
	}
	return validated, nil
}

// ValidateActionList validates typed actions.
func ValidateActionList(actions []Action) ([]Action, error) {
	raw := make([]map[string]any, len(actions))
	for i, action := range actions {
		raw[i] = map[string]any{"at": action.At, "pos": action.Pos}
	}
	return ValidateActions(raw)
}

func validateAction(action map[string]any, index int) (Action, error) {
	if action == nil {
		return Action{}, validationErrorf("action at index %d must be an object.", index)
	}
	atRaw, ok := action["at"]
	if !ok {
		return Action{}, validationErrorf("action at index %d missing 'at' timestamp.", index)
	}
	posRaw, ok := action["pos"]
	if !ok {
		return Action{}, validationErrorf("action at index %d missing 'pos' value.", index)
	}
	at, err := toFloat(atRaw)
	if err != nil {
		return Action{}, validationErrorf("action at index %d has invalid 'at' or 'pos': %v", index, action)
	}
	pos, err := toFloat(posRaw)
	if err != nil {
		return Action{}, validationErrorf("action at index %d has invalid 'at' or 'pos': %v", index, action)
	}
	atMS := int(at + 0.5)
	if atMS < 0 {
		return Action{}, validationErrorf("action at index %d has negative timestamp %d.", index, atMS)
	}
	if atMS > maxTimestampMS {
		return Action{}, validationErrorf("action at index %d timestamp %d exceeds limit (%d ms).", index, atMS, maxTimestampMS)
	}
	if pos < minPos || pos > maxPos {
		return Action{}, validationErrorf("action at index %d pos %v out of range [%d, %d].", index, pos, minPos, maxPos)
	}
	return Action{At: atMS, Pos: pos}, nil
}
