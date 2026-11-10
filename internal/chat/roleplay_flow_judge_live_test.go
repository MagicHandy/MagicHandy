//go:build liveeval

package chat

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// TestRoleplayFlowJudge reads a TestHostedRoleplayFlowLive capture from
// MAGICHANDY_ROLEPLAY_ROWS and writes MAGICHANDY_ROLEPLAY_JUDGED. Each reply is
// labelled from its text alone, without the motion data, so the labels can be
// compared with the compiled change afterwards. Each scene is then rated as a
// whole with the motion data. Run it on one fixed judge model for every run.
func TestRoleplayFlowJudge(t *testing.T) {
	in, out := os.Getenv("MAGICHANDY_ROLEPLAY_ROWS"), os.Getenv("MAGICHANDY_ROLEPLAY_JUDGED")
	if in == "" || out == "" {
		t.Skip("set MAGICHANDY_ROLEPLAY_ROWS and MAGICHANDY_ROLEPLAY_JUDGED")
	}
	provider, model := hostedLayeredLiveProvider(t)
	raw, err := os.ReadFile(in) // #nosec G304 -- explicit evaluation input.
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Turns []map[string]any `json:"turns"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	if len(capture.Turns) == 0 {
		t.Fatal("capture contains no turns")
	}
	scenes, order := map[string][]map[string]any{}, []string{}
	for _, row := range capture.Turns {
		scene, _ := row["scene"].(string)
		if _, seen := scenes[scene]; !seen {
			order = append(order, scene)
		}
		scenes[scene] = append(scenes[scene], row)
		if valid, ok := row["valid"].(bool); ok && !valid {
			row["judge_error"] = "request failed; no accepted reply to judge"
			t.Errorf("source turn failed for %s/%v; retained without a reply label", scene, row["turn"])
			continue
		}
		label, err := judgeRoleplayTurn(t, provider, model, row)
		if err != nil {
			row["judge_error"] = err.Error()
			t.Errorf("turn label failed for %s/%v: %v", scene, row["turn"], err)
			continue
		}
		row["judge"] = label
	}
	ratings := map[string]any{}
	for _, scene := range order {
		rating, err := judgeRoleplayScene(t, provider, model, scenes[scene])
		if err != nil {
			ratings[scene] = map[string]any{"error": err.Error()}
			t.Errorf("scene rating failed for %s: %v", scene, err)
			continue
		}
		ratings[scene] = rating
		t.Logf("%s %v", scene, rating)
	}
	encoded, err := json.MarshalIndent(map[string]any{"judge_model": model, "turns": capture.Turns, "scenes": ratings}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, encoded, 0600); err != nil {
		t.Fatal(err)
	}
}

const roleplayTurnJudgePrompt = `You annotate one turn of an adult roleplay. A partner character answers the person and also controls a stroking device that moves along one stroke, from the tip down to the base. Read only the partner's reply and report what it says or clearly implies about the physical motion right now. Do not guess what the device did.
The device stands for stroking: a hand, mouth or body moving on the person's genitals. Kissing, holding or touching elsewhere is not stroking.
onset: "starts" if stroking begins now after not happening; "stops" if stroking ends or pauses now, including resting or holding still together; otherwise "none".
pace: "faster" or "slower" if the reply says or clearly implies the stroking speeds up or slows down now; "steady" if it says the stroking keeps the same pace; otherwise "unstated".
depth: "deeper" if the stroking goes deeper, takes more, or goes further down toward the base now; "shallower" if it stays near the tip or takes less now; otherwise "unstated".
length: "longer" or "shorter" if the strokes get longer or shorter now; otherwise "unstated".
withholding: true if the reply teases by holding back, delaying or denying.
later_only: true if the reply only promises a change later, with none now.
Return only the JSON object.`

var roleplayTurnJudgeSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["onset","pace","depth","length","withholding","later_only"],"properties":{"onset":{"type":"string","enum":["starts","stops","none"]},"pace":{"type":"string","enum":["faster","slower","steady","unstated"]},"depth":{"type":"string","enum":["deeper","shallower","unstated"]},"length":{"type":"string","enum":["longer","shorter","unstated"]},"withholding":{"type":"boolean"},"later_only":{"type":"boolean"}}}`)

func judgeRoleplayTurn(t *testing.T, provider llm.Provider, model string, row map[string]any) (map[string]any, error) {
	user := fmt.Sprintf("Person: %v\nPartner's reply: %v", row["message"], row["reply"])
	return judgeRoleplayJSON(t, provider, model, roleplayTurnJudgePrompt, user, roleplayTurnJudgeSchema)
}

var roleplayStyleNotes = map[string]string{
	"neutral":    "no particular style",
	"playful":    "playful: light, quick and mischievous",
	"tender":     "tender: attentive, reassuring and unhurried",
	"dominant":   "dominant: leads, says what comes next and expects to be followed",
	"submissive": "submissive: follows the person's lead, asks and offers",
	"teasing":    "teasing: draws things out, withholds a little and enjoys anticipation",
}

const roleplaySceneJudgePrompt = `You review an adult roleplay scene. A partner character answers the person and also controls a stroking device. For each turn you get the person's message, the partner's reply, and what the device actually did, taken from the motion data rather than from the reply. Rate the scene from 1 (poor) to 5 (excellent):
coherence: the replies and the device tell the same story. A reply that describes starting, stopping, going faster, slower or deeper is matched by the device, and the device does not change in ways the replies contradict or never acknowledge.
responsiveness: the device responds to the person's cues, such as an invitation, pleasure, asking for more, getting close or winding down, instead of staying static or changing at random.
character: the motion choices fit the partner's style.
Return only the JSON object, with a one-sentence note on the main weakness.`

var roleplaySceneJudgeSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["coherence","responsiveness","character","note"],"properties":{"coherence":{"type":"integer","minimum":1,"maximum":5},"responsiveness":{"type":"integer","minimum":1,"maximum":5},"character":{"type":"integer","minimum":1,"maximum":5},"note":{"type":"string"}}}`)

func judgeRoleplayScene(t *testing.T, provider llm.Provider, model string, rows []map[string]any) (map[string]any, error) {
	sort.SliceStable(rows, func(i, j int) bool { return roleplayNumber(rows[i]["turn"]) < roleplayNumber(rows[j]["turn"]) })
	var builder strings.Builder
	style, _ := rows[0]["style"].(string)
	fmt.Fprintf(&builder, "Partner style: %s. Voice: %v.\n", roleplayStyleNotes[style], rows[0]["voice"])
	for _, row := range rows {
		reply := row["reply"]
		if valid, ok := row["valid"].(bool); ok && !valid {
			reply = "Request failed; no reply delivered."
		}
		fmt.Fprintf(&builder, "\nTurn %v\nPerson: %v\nPartner: %v\nDevice: %s\n", row["turn"], row["message"], reply, describeRoleplayChange(row))
	}
	return judgeRoleplayJSON(t, provider, model, roleplaySceneJudgePrompt, builder.String(), roleplaySceneJudgeSchema)
}

// describeRoleplayChange states the compiled change in plain words for the
// scene judge.
func describeRoleplayChange(row map[string]any) string {
	before, _ := row["running_before"].(bool)
	after, _ := row["running_after"].(bool)
	switch {
	case !before && after:
		return "started"
	case before && !after:
		return "stopped"
	case !before && !after:
		return "stays stopped"
	}
	feelBefore, feelAfter := roleplayFeelOf(row["feel_before"]), roleplayFeelOf(row["feel_after"])
	if feelBefore == nil || feelAfter == nil {
		return "running"
	}
	parts := []string{}
	if feelBefore["travel"] > 0 {
		change := (feelAfter["travel"] - feelBefore["travel"]) / feelBefore["travel"] * 100
		if change >= 10 || change <= -10 {
			parts = append(parts, fmt.Sprintf("pace %+.0f%%", change))
		}
	}
	if delta := feelAfter["min"] - feelBefore["min"]; delta <= -5 || delta >= 5 {
		word := "deeper"
		if delta > 0 {
			word = "shallower"
		}
		parts = append(parts, fmt.Sprintf("reaches %s (lowest point %.0f to %.0f, 0 is the base)", word, feelBefore["min"], feelAfter["min"]))
	}
	if feelBefore["stroke"] > 0 {
		change := (feelAfter["stroke"] - feelBefore["stroke"]) / feelBefore["stroke"] * 100
		if change >= 15 || change <= -15 {
			parts = append(parts, fmt.Sprintf("strokes %+.0f%% in length", change))
		}
	}
	if len(parts) == 0 {
		return "continues unchanged"
	}
	return strings.Join(parts, ", ")
}

// roleplayFeelOf accepts a feel recorded in this process or decoded from JSON.
func roleplayFeelOf(value any) map[string]float64 {
	switch feel := value.(type) {
	case map[string]float64:
		return feel
	case map[string]any:
		out := map[string]float64{}
		for key, number := range feel {
			out[key] = roleplayNumber(number)
		}
		return out
	}
	return nil
}

func roleplayNumber(value any) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case int:
		return float64(number)
	}
	return 0
}

func judgeRoleplayJSON(t *testing.T, provider llm.Provider, model, system, user string, schema json.RawMessage) (map[string]any, error) {
	raw, err := provider.StreamChat(t.Context(), llm.ChatRequest{Model: model, MaxTokens: 400, JSONSchema: schema,
		Messages: []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}}, nil)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &decoded); err != nil {
		return nil, fmt.Errorf("judge returned non-JSON: %w", err)
	}
	if err := validateRoleplayJudgment(decoded, schema); err != nil {
		return nil, err
	}
	return decoded, nil
}

// These two evaluation schemas contain only required scalar fields. Validate
// their types and bounds even when a provider does not enforce JSONSchema.
func validateRoleplayJudgment(decoded map[string]any, schema json.RawMessage) error {
	var contract struct {
		Properties map[string]struct {
			Type             string
			Enum             []string
			Minimum, Maximum float64
		}
	}
	if err := json.Unmarshal(schema, &contract); err != nil {
		return err
	}
	if len(decoded) != len(contract.Properties) {
		return fmt.Errorf("judge returned missing or extra fields")
	}
	for key, field := range contract.Properties {
		value, ok := decoded[key]
		if !ok || value == nil {
			return fmt.Errorf("judge omitted %s", key)
		}
		valid := false
		switch field.Type {
		case "string":
			text, ok := value.(string)
			valid = ok && strings.TrimSpace(text) != "" && (len(field.Enum) == 0 || slices.Contains(field.Enum, text))
		case "boolean":
			_, valid = value.(bool)
		case "integer":
			number, ok := value.(float64)
			valid = ok && math.Trunc(number) == number && number >= field.Minimum && number <= field.Maximum
		}
		if !valid {
			return fmt.Errorf("judge returned invalid %s", key)
		}
	}
	return nil
}
