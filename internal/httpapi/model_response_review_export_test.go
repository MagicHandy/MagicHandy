//go:build liveeval && magichandy_labs

package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Export retained model selections through the actual dispatch conversion and
// shared compiler, including original Creative and Pattern library starts.
// This is inert: no Server, engine, transport or provider is constructed.
func TestExportModelResponseReview(t *testing.T) {
	inputs, output := os.Getenv("MAGICHANDY_REVIEW_INPUTS"), os.Getenv("MAGICHANDY_REVIEW_OUTPUT")
	if inputs == "" || output == "" {
		t.Skip("explicit retained response reports and an ignored output path are required")
	}
	turns := []map[string]any{}
	for _, path := range strings.Split(inputs, ",") {
		data, err := os.ReadFile(path) // #nosec G304 -- opt-in local evaluation input.
		if err != nil {
			t.Fatal(err)
		}
		var report struct{ Turns []modelReviewRow }
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		for index, row := range report.Turns {
			name := filepath.Base(path) + "/" + row.RecipeName
			if row.RecipeName == "" {
				name = fmt.Sprintf("%s/turn-%d", filepath.Base(path), index+1)
			}
			review := compileModelReview(row)
			review.ID, review.Name, review.Model = fmt.Sprintf("%s/%04d", filepath.Base(path), index+1), name, row.Model
			review.Request, review.Raw = row.Message, row.Raw
			turns = append(turns, map[string]any{"request": row.Message, "response": row.Raw, "intent_pass": row.IntentPass, "motion": review})
		}
	}
	data, err := json.Marshal(map[string]any{"inert": true, "model": "retained model evaluations", "turns": turns})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("retained %d selections including failures and inert replies", len(turns))
}

type modelReviewRow struct {
	Model, Method, Message, Raw, Error string
	RecipeName                         string `json:"recipe_name"`
	IntentPass                         bool   `json:"intent_pass"`
	Valid                              bool
	Limits                             config.MotionSettings
	After                              *motion.FlowSpec
	MotionCommand                      *chat.MotionCommand `json:"motion_command"`
	MotionUnchanged                    bool                `json:"motion_unchanged"`
	Paused                             bool
}

func compileModelReview(row modelReviewRow) motion.Review {
	if !row.Valid {
		return motion.Review{Error: "Rejected response: " + row.Error}
	}
	var envelope struct{ Action string }
	_ = json.Unmarshal([]byte(row.Raw), &envelope)
	if row.MotionUnchanged || row.Paused || envelope.Action == chat.MotionActionNone {
		return motion.Review{Error: "No new motion proposal; existing state retained"}
	}
	if row.MotionCommand != nil {
		command := row.MotionCommand
		if command.Action == chat.MotionActionNone || command.Action == chat.MotionActionStop {
			return motion.Review{Error: "No moving command: " + command.Action}
		}
		if row.Method == string(chat.MotionModeDynamic) {
			return motion.ReviewMotionOutput(dynamicChatMotionTarget(command, motion.ActiveMotionState{}), row.Limits)
		}
		if row.Method == string(chat.MotionModePattern) {
			if command.PatternID == "" || command.SpeedPercent == nil {
				return motion.Review{Error: "Accepted start with implicit content; no current library target captured"}
			}
			return motion.ReviewMotionOutput(motion.MotionTarget{PatternID: motion.PatternID(command.PatternID), SpeedPercent: *command.SpeedPercent}, row.Limits)
		}
	}
	if row.After != nil {
		target, err := motion.FlowTarget(*row.After, row.Limits)
		if err != nil {
			return motion.Review{Error: err.Error()}
		}
		return motion.ReviewMotionOutput(target, row.Limits)
	}
	return motion.Review{Error: "No accepted motion command"}
}
