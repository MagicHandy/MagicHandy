//go:build liveeval && magichandy_labs

package httpapi

import (
	"os"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLiveSavedUserHistory(t *testing.T) {
	name := os.Getenv("MAGICHANDY_QUALITY_PROFILE")
	if name == "" {
		t.Skip("select a signed-in Terra or installed default Gemma profile")
	}
	profile := loadQualityDefault(t, name)
	if name == "terra" {
		profile.settings.Connections[0].ContextWindow = 65536
		profile.window, profile.messages, _ = profile.settings.Connections[0].ConversationWindow()
	}
	active := profile.settings.ConversationSettings()
	for _, scope := range []string{"complete", "partial", "unavailable"} {
		t.Run(scope, func(t *testing.T) {
			s, p, fake := qualityServer(t, active.Model, config.LLMMotionModeLayered, "", profile.window, active.IsHosted(), profile)
			appendUser := func(text string) {
				t.Helper()
				if _, err := s.chatLog.Append(chat.MessageRoleUser, text, "history fixture"); err != nil {
					t.Fatal(err)
				}
			}
			appendUser("For this conversation my checkpoint word is juniper. Remember it for later. Leave motion off for now.")
			if scope != "unavailable" {
				appendUser("Start steady full sweeps at exactly 25 percent pace. Keep the full stroke range.")
			}
			if scope == "complete" {
				appendUser("Keep the same pace, stroke range, and pattern. In one short sentence tell me the current pace.")
				appendUser("Change only the pace to 35 percent. Keep those same full sweeps.")
				appendUser("Five percentage points faster than that. Leave every other setting alone.")
			} else {
				appendUser(strings.Repeat("a long saved instruction ", 200))
			}
			qualityFillHistory(t, s, profile.messages/2+2)
			appendUser("Change the pace to 30 percent. Call that my baseline.")
			appendUser("Now set the pace to 38 percent.")
			events, _ := qualityChat(t, s, "Keep motion off. From the very beginning of this conversation, what were the first two distinct paces I explicitly asked you to set, and by how many percentage points did I then ask you to increase the second? Give the three numbers in order only if you can find them in the conversation. Otherwise say which are unavailable.")
			t.Logf("%s %s: %q (%d provider calls)", name, scope, events.Reply, len(p.attempts))
			for _, attempt := range p.attempts {
				t.Logf("provider input %d bytes, output %s", attempt.InputBytes, attempt.Raw)
			}
			if !events.OK || events.Repaired || events.Fallback || len(fake.Commands()) != 0 || events.Action != "none" {
				t.Fatal("history question did not complete as a text-only turn")
			}
			answer := strings.ToLower(events.Reply)
			if strings.Contains(answer, "38") || strings.Contains(answer, "30") ||
				(scope == "unavailable" && answer != "unavailable") ||
				(scope == "partial" && (strings.Count(answer, "25") != 1 || strings.Count(answer, "unavailable") != 2)) ||
				strings.Contains(answer, "25") != (scope != "unavailable") ||
				strings.Contains(answer, "35") != (scope == "complete") ||
				(scope == "complete" && !strings.Contains(answer, "five") && !strings.Contains(answer, ", 5")) ||
				strings.Contains(answer, "unavailable") != (scope != "complete") {
				t.Fatal("reply disagrees with available opening evidence")
			}
			if scope == "complete" {
				named, _ := qualityChat(t, s, "Keep motion off. What was the checkpoint word, and what number did I name my baseline?")
				t.Logf("named details: %q", named.Reply)
				if !named.OK || named.Repaired || named.Fallback || len(fake.Commands()) != 0 ||
					!strings.Contains(strings.ToLower(named.Reply), "juniper") || !strings.Contains(named.Reply, "30") {
					t.Fatal("named details did not come from the original user messages")
				}
				p.attempts = nil
				control, _ := qualityChat(t, s, "Start steady full sweeps now at exactly 42 percent pace.")
				state := qualityState(s)
				t.Logf("current request: %q, running=%t, pace=%d (%d provider calls)", control.Reply, state.Running, state.Target.SpeedPercent, len(p.attempts))
				if !control.OK || control.Repaired || control.Fallback || !state.Running || state.Target.SpeedPercent != 42 {
					t.Fatal("current request did not reach the shared engine")
				}
				qualityChat(t, s, "Stop.")
			}
		})
	}
}
