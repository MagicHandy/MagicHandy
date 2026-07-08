package manualqueue

import "sort"

// Action is one funscript keyframe.
type Action struct {
	At  int `json:"at"`
	Pos int `json:"pos"`
}

// Item is one manual queue slot.
type Item struct {
	BlockID     string
	DurationSec float64
	Loop        bool
}

// NormalizeActionsToZero shifts the first keyframe to t=0.
func NormalizeActionsToZero(actions []Action) []Action {
	if len(actions) == 0 {
		return nil
	}
	sorted := append([]Action(nil), actions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].At < sorted[j].At })
	base := sorted[0].At
	out := make([]Action, len(sorted))
	for i, action := range sorted {
		out[i] = Action{At: action.At - base, Pos: action.Pos}
	}
	return out
}

// ExpandBlockActions expands one block to fill durationSec.
func ExpandBlockActions(actions []Action, durationSec float64, loop bool) []Action {
	targetMS := int(durationSec * 1000)
	if targetMS < 1 {
		targetMS = 1
	}
	norm := NormalizeActionsToZero(actions)
	if len(norm) < 2 {
		return norm
	}
	cycleMS := norm[len(norm)-1].At
	if cycleMS < 1 {
		cycleMS = 1
	}
	if loop {
		out := make([]Action, 0, len(norm)*2)
		offset := 0
		for offset < targetMS {
			for _, item := range norm {
				at := offset + item.At
				if at > targetMS {
					break
				}
				out = append(out, Action{At: at, Pos: item.Pos})
			}
			offset += cycleMS
		}
		return out
	}
	trimmed := make([]Action, 0, len(norm))
	for _, item := range norm {
		if item.At <= targetMS {
			trimmed = append(trimmed, item)
		}
	}
	if len(trimmed) > 0 && trimmed[len(trimmed)-1].At < targetMS {
		last := trimmed[len(trimmed)-1]
		trimmed = append(trimmed, Action{At: targetMS, Pos: last.Pos})
	}
	return trimmed
}

// ConcatManualQueueItems concatenates queue slots into one action list.
func ConcatManualQueueItems(items []Item, blocks map[string][]Action) ([]Action, int) {
	merged := make([]Action, 0, 128)
	offsetMS := 0
	for _, item := range items {
		slotMS := int(item.DurationSec * 1000)
		if slotMS < 1 {
			slotMS = 1
		}
		segment := ExpandBlockActions(blocks[item.BlockID], item.DurationSec, item.Loop)
		for _, action := range segment {
			merged = append(merged, Action{
				At:  offsetMS + action.At,
				Pos: action.Pos,
			})
		}
		offsetMS += slotMS
	}
	return merged, offsetMS
}

// SegmentStarts returns the start offset in ms for each queue slot.
func SegmentStarts(items []Item) []int {
	starts := make([]int, len(items))
	offset := 0
	for i, item := range items {
		starts[i] = offset
		slotMS := int(item.DurationSec * 1000)
		if slotMS < 1 {
			slotMS = 1
		}
		offset += slotMS
	}
	return starts
}

// SegmentIndexAt returns the segment index for a playhead position.
func SegmentIndexAt(segmentStarts []int, playheadMS int) int {
	if len(segmentStarts) == 0 {
		return 0
	}
	index := 0
	for i, start := range segmentStarts {
		if playheadMS >= start {
			index = i
		}
	}
	return index
}
