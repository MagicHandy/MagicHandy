package funscript

// NormalizeActions sorts by timestamp and collapses duplicate timestamps.
func NormalizeActions(actions []Action) []Action {
	if len(actions) == 0 {
		return nil
	}

	sorted := append([]Action(nil), actions...)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].At < sorted[i].At {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	deduped := []Action{{At: sorted[0].At, Pos: sorted[0].Pos}}
	for _, action := range sorted[1:] {
		last := &deduped[len(deduped)-1]
		if action.At == last.At {
			last.Pos = action.Pos
			continue
		}
		deduped = append(deduped, Action{At: action.At, Pos: action.Pos})
	}
	return deduped
}
