//go:build liveeval && magichandy_labs

package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Exercise the actual chat, button and configured planning provider using a
// temporary store and simulator. The baseline removes only the observed event.
func TestLiveEdgeConversation(t *testing.T) {
	name := os.Getenv("MAGICHANDY_QUALITY_PROFILE")
	if name == "" || os.Getenv("MAGICHANDY_EDGE_FEEDBACK_EVAL") == "" {
		t.Skip("select a live Edge review profile")
	}
	var profile *qualityDefaultProfile
	if model := os.Getenv("MAGICHANDY_EDGE_HOSTED_MODEL"); name == "terra" && model != "" {
		profile = qualityChatGPTProfile(t, model)
	} else {
		profile = loadQualityDefault(t, name)
	}
	active := profile.settings.ConversationSettings()
	s, p, _ := qualityServer(t, active.Model, config.LLMMotionModeLayered, "", profile.window, active.IsHosted(), profile)
	baseline := os.Getenv("MAGICHANDY_EDGE_BASELINE") != ""
	post := func(path, body string) {
		t.Helper()
		if res := personalizationRequest(t, s, http.MethodPost, path, body); res.Code != http.StatusOK {
			t.Fatalf("%s: %s", path, res.Body.String())
		}
	}
	post("/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":50}`)
	post("/api/motion/edge", `{}`)
	before := qualityState(s)
	if !before.Running || before.Target.Source != "edge" {
		t.Fatal("button did not install hold")
	}
	if baseline {
		if err := s.store.Datastore().WithTx(t.Context(), func(tx *sql.Tx) error { _, err := tx.Exec("DELETE FROM chat_edge_holds"); return err }); err != nil {
			t.Fatal(err)
		}
	}
	var rows []map[string]any
	for _, tc := range []struct{ name, text string }{
		{"reaction", "That was too close. I'd like more time to settle on the next Edge."},
		{"specific", "Next time I press Edge, hold for 76 seconds and return at 24 percent. Just acknowledge for now."},
		{"readback", "What holding pace did my last press of the Edge button set? Just give the number. Keep doing what you're doing."},
		{"normal", "What color do you get when you mix blue and yellow paint? Just tell me."},
	} {
		prev := len(p.attempts)
		events, stream := qualityChat(t, s, tc.text)
		if !events.OK {
			for _, block := range strings.Split(strings.ReplaceAll(stream, "\r\n", "\n"), "\n\n") {
				if strings.HasPrefix(block, "event: error\n") {
					t.Logf("%s: %s", tc.name, block)
				}
			}
		}
		state := qualityState(s)
		pass := events.deliveredReply() && !events.Repaired && events.Action == "none" && state.PlanID == before.PlanID
		if tc.name == "readback" {
			pass = pass && strings.Contains(events.Reply, "20")
		}
		if tc.name == "normal" {
			pass = pass && strings.Contains(strings.ToLower(events.Reply), "green")
		}
		rows = append(rows, map[string]any{"case": tc.name, "pass": pass, "reply": events.Reply, "action": events.Action, "calls": len(p.attempts) - prev, "repaired": events.Repaired, "fallback": events.Fallback})
	}
	if edge := s.modes.Status().EdgeHold; edge != nil && edge.Active {
		res := personalizationRequest(t, s, http.MethodPost, "/api/motion/edge/resume", `{}`)
		if res.Code != http.StatusOK {
			t.Log("moving hold was replaced during chat", res.Body.String())
		}
	}
	// Starting Autopilot over the restored simulation and pressing again uses
	// the ordinary mode tick, provider admission, validation and countdown.
	post("/api/modes/start", `{"mode":"autopilot"}`)
	post("/api/motion/edge", `{}`)
	var seconds, pace int
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && seconds == 0 {
		for _, row := range s.traces.Rows() {
			if row.Planner != nil && row.Planner.Event == "edge_return_planned" {
				seconds = int(row.Planner.DurationMillis / 1000)
				pace = row.Planner.SpeedPercent
			}
		}
		if seconds == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	rows = append(rows, map[string]any{"case": "next_hold", "pass": seconds == 76 && pace == 24, "planned_seconds": seconds, "planned_return_pace": pace})
	events, _ := qualityChat(t, s, "Change the current pace to 42 percent now. Keep the same range.")
	state := qualityState(s)
	rows = append(rows, map[string]any{"case": "current_edit", "pass": events.deliveredReply() && !events.Repaired && state.Target.SpeedPercent == 42, "reply": events.Reply, "action": events.Action, "pace": state.Target.SpeedPercent})
	qualityChat(t, s, "Stop.")
	if qualityState(s).Running {
		t.Fatal("still running after Stop")
	}
	enc, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE"); path != "" {
		if err = os.WriteFile(path, enc, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range rows {
		t.Logf("%s: %v", row["case"], row)
		if os.Getenv("MAGICHANDY_EDGE_REQUIRE_QUALITY") != "" && row["pass"] != true {
			t.Errorf("quality miss for %s", row["case"])
		}
	}
}
