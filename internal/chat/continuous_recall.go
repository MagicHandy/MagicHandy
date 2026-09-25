package chat

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// EarlierScore is a distinct score that played earlier in this Autopilot
// session, newest first in a MotionContext.
type EarlierScore struct {
	Flow              motion.FlowSpec
	StartedSecondsAgo int
	PlayedSeconds     int
}

// earlierScoresGuide is shown only while there are earlier scores to recall.
// Recall answers a person who praises or asks for something that played
// before; it is not a way to repeat by default.
const earlierScoresGuide = `earlier_scores lists distinct scores that played earlier, newest first, with when each started, how long it played and what the person said while it played. When they praise or ask for something that was playing earlier, the edit {"recall": id} brings that score back exactly, and other edits in the same reply apply on top of it. Praise for what is playing now needs no recall.`

// recallableScores keeps only earlier scores the current mode can play.
func recallableScores(state MotionContext) []EarlierScore {
	creative := state.MotionMode == MotionModeCreativeV2
	scores := make([]EarlierScore, 0, len(state.EarlierScores))
	for _, score := range state.EarlierScores {
		if (score.Flow.Gesture != nil) == creative {
			scores = append(scores, score)
		}
	}
	return scores
}

type earlierScoreContext struct {
	ID                int      `json:"id"`
	StartedSecondsAgo int      `json:"started_seconds_ago"`
	PlayedSeconds     int      `json:"played_seconds"`
	SaidWhilePlaying  []string `json:"said_while_playing,omitempty"`
	Score             any      `json:"score"`
}

func earlierScoresContext(scores []EarlierScore, state MotionContext) []earlierScoreContext {
	out := make([]earlierScoreContext, len(scores))
	for i, score := range scores {
		var described any = layeredScoreContext(score.Flow)
		if state.MotionMode == MotionModeCreativeV2 {
			described = creativeV2ScoreContext(score.Flow)
		}
		out[i] = earlierScoreContext{ID: i + 1, StartedSecondsAgo: score.StartedSecondsAgo, PlayedSeconds: score.PlayedSeconds,
			SaidWhilePlaying: saidWhilePlaying(score, state), Score: described}
	}
	return out
}

// saidWhilePlaying lists the person's lines said after the score started and
// no later than it ended. A line that made chat replace the score was said
// while the old score played.
func saidWhilePlaying(score EarlierScore, state MotionContext) []string {
	if len(state.UserRequestSecondsAgo) != len(state.UserRequests) {
		return nil
	}
	var said []string
	for i, line := range state.UserRequests {
		age := state.UserRequestSecondsAgo[i]
		if age < score.StartedSecondsAgo && age >= score.StartedSecondsAgo-score.PlayedSeconds {
			said = append(said, line)
		}
	}
	return said
}

// withRecallSchema offers {"recall": id} as one more edit when earlier scores
// exist. Creative v2 edits are a list of one-group items; Layered edits are
// one object.
func withRecallSchema(schema json.RawMessage, mode MotionMode, count int) json.RawMessage {
	if count == 0 || len(schema) == 0 {
		return schema
	}
	recall := map[string]any{"type": "integer", "minimum": 1, "maximum": count}
	var root map[string]any
	if json.Unmarshal(schema, &root) != nil {
		return schema
	}
	properties, _ := root["properties"].(map[string]any)
	edits, _ := properties["edits"].(map[string]any)
	if edits == nil {
		return schema
	}
	if mode == MotionModeCreativeV2 {
		items, _ := edits["items"].(map[string]any)
		choices, _ := items["oneOf"].([]any)
		if items == nil {
			return schema
		}
		items["oneOf"] = append(choices, map[string]any{"type": "object", "properties": map[string]any{"recall": recall},
			"required": []string{"recall"}, "additionalProperties": false})
	} else {
		fields, _ := edits["properties"].(map[string]any)
		if fields == nil {
			return schema
		}
		fields["recall"] = recall
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return schema
	}
	return encoded
}

// applyRecall takes a recall edit out of a reply and returns the score it
// names as the base for the remaining edits. The parsers then see an ordinary
// reply. A recalled pace outside today's saved limits moves to the nearest one.
func applyRecall(raw string, current motion.FlowSpec, scores []EarlierScore, mode MotionMode, speedMin, speedMax int) (string, motion.FlowSpec, bool, error) {
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &root) != nil || len(root["edits"]) == 0 {
		return raw, current, false, nil
	}
	id, edits, found, err := takeRecall(root["edits"], mode)
	if err != nil || !found {
		return raw, current, false, err
	}
	if id < 1 || id > len(scores) {
		return raw, current, false, fmt.Errorf("recall %d does not name an earlier score", id)
	}
	base := *motion.CloneFlowSpec(&scores[id-1].Flow)
	if speedMax > 0 {
		base.SpeedPercent = min(max(base.SpeedPercent, speedMin), speedMax)
	}
	root["edits"] = edits
	encoded, _ := json.Marshal(root)
	return string(encoded), base, true, nil
}

func takeRecall(edits json.RawMessage, mode MotionMode) (int, json.RawMessage, bool, error) {
	var id int
	if mode == MotionModeCreativeV2 {
		var items []map[string]json.RawMessage
		if json.Unmarshal(edits, &items) != nil {
			return 0, edits, false, nil
		}
		kept := make([]map[string]json.RawMessage, 0, len(items))
		found := false
		for _, item := range items {
			value, ok := item["recall"]
			if !ok {
				kept = append(kept, item)
				continue
			}
			if found || len(item) != 1 || json.Unmarshal(value, &id) != nil {
				return 0, edits, false, errors.New("recall must be a single edit naming one earlier score")
			}
			found = true
		}
		encoded, _ := json.Marshal(kept)
		return id, encoded, found, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(edits, &fields) != nil {
		return 0, edits, false, nil
	}
	value, ok := fields["recall"]
	if !ok {
		return 0, edits, false, nil
	}
	if json.Unmarshal(value, &id) != nil {
		return 0, edits, false, errors.New("recall must name one earlier score")
	}
	delete(fields, "recall")
	encoded, _ := json.Marshal(fields)
	return id, encoded, true, nil
}
