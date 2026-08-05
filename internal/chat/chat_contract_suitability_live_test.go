package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Ranks a model for THIS app, not for prose. MagicHandy drives a physical device
// through a JSON contract, so contract compliance outranks writing quality: a
// model that writes beautifully but cannot emit a well-formed motion object is
// unusable, and the register harness cannot see that because replyOf() digs the
// reply out of whatever arrived and discards the rest.
//
//	LLAMACPP=http://127.0.0.1:8792 MODEL=name go test ./internal/chat -run TestSuitability -v
//
// Results and the resulting ranking live in docs/model-suitability.md.

type intent struct {
	turn string
	// want is what the contract should carry for this turn, or "" when the turn
	// is conversational and motion is optional.
	want string
}

var suitabilityTurns = []intent{
	{"Fuck me.", "start"},
	{"Slower please", "slower"},
	{"Faster", "faster"},
	{"Just the tip", "tip"},
	{"Tell me what you're thinking", "chat"},
	{"Go back to the whole range", "full"},
	{"Harder", "faster"},
	{"Talk dirty to me", "chat"},
	{"Stop", "stop"},
	{"How does that feel to you", "chat"},
}

type contractShape struct {
	Reply  string          `json:"reply"`
	Motion *motionShape    `json:"motion"`
	Action json.RawMessage `json:"action"`
	Speed  json.RawMessage `json:"speed_percent"`
	Patt   json.RawMessage `json:"pattern_id"`
}

type motionShape struct {
	Action    string  `json:"action"`
	Speed     *int    `json:"speed_percent"`
	PatternID string  `json:"pattern_id"`
	Intensity *int    `json:"intensity"`
	Area      string  `json:"area"`
	Extra     float64 `json:"-"`
}

type suitability struct {
	turns, jsonOK, replyOK, leak, intentOK, intentTried int
	pairViolations, rangeViolations                     int
	totalMillis, totalWords                             int
	register                                            *score
}

func firstJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ""
	}
	return raw[start : end+1]
}

var mFence = regexp.MustCompile("(?s)```")

func judgeIntent(want string, shape contractShape, prevSpeed int) (tried bool, ok bool, speed int) {
	m := shape.Motion
	speed = prevSpeed
	switch want {
	case "chat":
		// Conversational turns must not invent a motion change. "none" is fine.
		return true, m == nil || m.Action == "none", speed
	case "stop":
		return true, m != nil && m.Action == "stop", speed
	case "start":
		return true, m != nil && (m.Action == "start" || m.Action == "target"), speedOf(m, speed)
	case "tip":
		return true, m != nil && m.Area == "tip", speedOf(m, speed)
	case "full":
		return true, m != nil && m.Area == "full", speedOf(m, speed)
	case "slower":
		if m == nil || m.Speed == nil {
			return true, false, speed
		}
		return true, *m.Speed < prevSpeed, *m.Speed
	case "faster":
		if m == nil || m.Speed == nil {
			return true, false, speed
		}
		return true, *m.Speed > prevSpeed, *m.Speed
	}
	return false, false, speed
}

func speedOf(m *motionShape, fallback int) int {
	if m != nil && m.Speed != nil {
		return *m.Speed
	}
	return fallback
}

// recordTurn folds one response into the tally and reports whether it produced a
// usable object. It exists so the test body stays a loop over turns rather than
// a single function carrying every contract rule.
func (s *suitability) recordTurn(t *testing.T, it intent, raw string, prevSpeed *int) (contractShape, bool) {
	t.Helper()
	var shape contractShape
	if strings.TrimSpace(raw) == "" {
		return shape, false
	}
	candidate := firstJSONObject(raw)
	parsed := candidate != "" && json.Unmarshal([]byte(candidate), &shape) == nil
	if parsed && !mFence.MatchString(raw) {
		s.jsonOK++
	}
	if !parsed {
		t.Logf("  [%s] BAD JSON: %s", it.turn, truncate(raw, 120))
		return shape, false
	}
	if strings.TrimSpace(shape.Reply) != "" {
		s.replyOK++
		s.register.add(shape.Reply)
		s.totalWords += len(strings.Fields(shape.Reply))
	}
	// Top-level action/speed_percent/pattern_id are explicitly forbidden by the
	// contract; they are the most common way a weak model breaks it.
	if len(shape.Action) > 0 || len(shape.Speed) > 0 || len(shape.Patt) > 0 {
		s.leak++
	}
	s.recordMotionShape(shape.Motion)

	tried, correct, next := judgeIntent(it.want, shape, *prevSpeed)
	if tried {
		s.intentTried++
		if correct {
			s.intentOK++
		} else {
			t.Logf("  [%s] want %s got %s", it.turn, it.want, motionSummary(shape.Motion))
		}
	}
	*prevSpeed = next
	return shape, true
}

// recordMotionShape counts the structural contract rules a motion object can
// break independently of whether it matched the turn's intent.
func (s *suitability) recordMotionShape(m *motionShape) {
	if m == nil {
		return
	}
	// pattern_id and intensity are an inseparable pair.
	if (m.PatternID != "") != (m.Intensity != nil) {
		s.pairViolations++
	}
	if m.Speed != nil && (*m.Speed < 0 || *m.Speed > 100) {
		s.rangeViolations++
	}
	if m.PatternID != "" && m.Speed != nil {
		s.pairViolations++ // must choose one pacing representation
	}
}

func TestSuitability(t *testing.T) {
	base := os.Getenv("LLAMACPP")
	if base == "" {
		t.Skip("set LLAMACPP")
	}
	endpoint := loopbackEndpoint(t, base)
	label := os.Getenv("MODEL")

	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	caps := FullCapabilities()
	caps.Voice = VoiceExplicit
	caps.Style = StyleDominant
	caps.MoodTracking = true
	ctx := &ConversationContext{
		PersonaName: "Mara", PersonaDescription: evalPersonas[0].desc,
		UserAnatomy: "penis", CurrentMood: "Intimate",
	}

	s := suitability{register: newScore()}
	prevSpeed := 30
	var history []omsg
	for _, it := range suitabilityTurns {
		system := composeSystem(set, nil, defaultPatternChoices(), caps, nil, ctx)
		began := time.Now()
		raw := matrixGenerate(t, endpoint, system, history, it.turn)
		elapsed := int(time.Since(began).Milliseconds())
		s.turns++
		s.totalMillis += elapsed
		shape, ok := s.recordTurn(t, it, raw, &prevSpeed)
		if !ok {
			continue
		}

		history = append(history, omsg{Role: "user", Content: it.turn}, omsg{Role: "assistant", Content: shape.Reply})
		if len(history) > 6 {
			history = history[len(history)-6:]
		}
		ctx.RecentAssistantReplies = append(ctx.RecentAssistantReplies, shape.Reply)
		if len(ctx.RecentAssistantReplies) > maxRecentAssistantReplies {
			ctx.RecentAssistantReplies = ctx.RecentAssistantReplies[1:]
		}
	}

	pct := func(v, of int) float64 {
		if of == 0 {
			return 0
		}
		return 100 * float64(v) / float64(of)
	}
	r := s.register
	expl, iSent := 0.0, 0.0
	if r.n > 0 {
		expl = 100 * float64(r.explicit) / float64(r.n)
	}
	if r.sentences > 0 {
		iSent = 100 * float64(r.iSentences) / float64(r.sentences)
	}
	fmt.Printf("\nSUIT %-42s json %3.0f%% reply %3.0f%% leak %d pair %d range %d | intent %3.0f%% (%d/%d) | ms/turn %4d words %3.0f | expl %3.0f%% I-sent %3.0f%% vocab %d\n",
		label,
		pct(s.jsonOK, s.turns), pct(s.replyOK, s.turns), s.leak, s.pairViolations, s.rangeViolations,
		pct(s.intentOK, s.intentTried), s.intentOK, s.intentTried,
		s.totalMillis/max1(s.turns), float64(s.totalWords)/float64(max1(r.n)),
		expl, iSent, len(r.vocab))
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

func motionSummary(m *motionShape) string {
	if m == nil {
		return "no motion"
	}
	out := m.Action
	if m.Speed != nil {
		out += fmt.Sprintf(" speed=%d", *m.Speed)
	}
	if m.PatternID != "" {
		out += " pattern=" + m.PatternID
	}
	if m.Area != "" {
		out += " area=" + m.Area
	}
	return out
}
