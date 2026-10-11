//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type reportProbeProvider struct {
	llm.Provider
	legacySpeech bool
	lastRaw      string
	baseline     string
	schema       bool
	request      llm.ChatRequest
	progress     llm.ProviderProgress
	calls        int
}

func (p *reportProbeProvider) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	if p.baseline != "" {
		request.Messages = append([]llm.Message(nil), request.Messages...)
		request.Messages[0].Content = p.baseline
	}
	if !p.schema {
		request.JSONSchema = nil
	}
	if p.legacySpeech {
		var schema map[string]any
		if json.Unmarshal(request.JSONSchema, &schema) == nil {
			delete(schema["properties"].(map[string]any), "intent")
			schema["required"] = []string{"reply", "next"}
			request.JSONSchema, _ = json.Marshal(schema)
		}
		request.Messages = append([]llm.Message(nil), request.Messages...)
		for index := range request.Messages {
			text := request.Messages[index].Content
			text = strings.ReplaceAll(text, autopilotSpeechDecisionContract, `The object requires a non-empty "reply" string and "next":"soon"|"normal"|"later".`)
			text = strings.ReplaceAll(text, `, or {"intent":"quiet","reply":"","next":"later"} when there is nothing useful to add. No motion.`, `. No motion.`)
			text = strings.ReplaceAll(text, `{"intent":"<what this line adds>","reply":`, `{"reply":`)
			text = strings.ReplaceAll(text, "The following style guidance applies only if you choose to speak; it does not require a reply.\n", "")
			text = strings.TrimSuffix(text, "\n"+autopilotSpeechDecisionInstruction)
			request.Messages[index].Content = text
		}
	}
	request.OnProgress = func(progress llm.ProviderProgress) { p.progress = progress }
	p.request = request
	p.calls++
	raw, err := p.Provider.StreamChat(ctx, request, delta)
	p.lastRaw = raw
	return raw, err
}

func writeReportProbe(t *testing.T, suffix string, value any) {
	t.Helper()
	path := os.Getenv("MAGICHANDY_REPORT_ISSUES_OUTPUT")
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+suffix+".json", data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReportIssuesLiveActionContract(t *testing.T) {
	model := os.Getenv("MAGICHANDY_REPORT_ISSUES_MODEL")
	if model == "" {
		t.Skip("set MAGICHANDY_REPORT_ISSUES_MODEL")
	}
	options := llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second}
	var native llm.Provider
	var err error
	compatible := os.Getenv("MAGICHANDY_REPORT_ISSUES_COMPATIBLE") != ""
	if compatible {
		native, err = llm.NewLlamaCPPProvider(options)
	} else {
		native, err = llm.NewOllamaProvider(options)
	}
	if err != nil {
		t.Fatal(err)
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	cases := []struct {
		message               string
		running, paused       bool
		speed, variation, min int
	}{
		{"Hello. Briefly explain what you can do; keep motion off.", false, false, 25, -1, -1},
		{"Start moving at 25 percent with the whole 0 to 100 range.", false, false, 25, -1, 0},
		{"Set speed to 30 percent and variation to 60 percent, preserving everything else.", true, false, 30, 60, -1},
		{"Use only the top half of the slider, 50 to 100, and set speed to 30 percent.", true, false, 30, -1, 50},
		{"Remove rebounds and set inertia to 70 percent. Preserve speed and reach.", true, false, 25, -1, -1},
		{"Keep this exact score unchanged and briefly describe it.", true, false, 25, -1, -1},
		{"Make it faster while it is paused.", true, true, 25, -1, -1},
		{"What does roaming change? Leave motion unchanged.", true, false, 25, -1, -1},
	}
	rows := []map[string]any{}
	for _, variant := range []string{"baseline", "candidate"} {
		for _, schema := range []bool{false, true} {
			if compatible && (variant == "baseline" || !schema) {
				continue
			}
			for _, reasoning := range []string{"off", "auto"} {
				for index, scenario := range cases {
					if reasoning == "auto" && index > 3 {
						continue
					}
					score := FreshCreativeV2Score(25)
					state := MotionContext{Running: scenario.running, Paused: scenario.paused, MotionMode: MotionModeCreativeV2, Layered: &score, SpeedMinPercent: 10, SpeedMaxPercent: 43, SpeedPercent: 25}
					capabilities := Capabilities{Motion: true, MotionMode: MotionModeCreativeV2, SchemaUnenforced: !schema}
					prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
					provider := &reportProbeProvider{Provider: native, schema: schema}
					if variant == "baseline" {
						original := composeSystem(prompt, nil, nil, capabilities, &state, nil)
						guard := continuousOutputGuard(capabilities)
						provider.baseline = strings.TrimSuffix(original, guard) + continuousActionGuideFor(state) + "\n\n" + guard
					}
					service := Service{Provider: provider, Prompt: prompt, Model: model, MaxTokens: 1536, ReasoningMode: reasoning, Capabilities: &capabilities, MotionContext: &state}
					started := time.Now()
					result, generationErr := service.Complete(t.Context(), Request{Message: scenario.message}, nil)
					after := score
					if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
						after = *result.Response.Motion.Layered
					}
					pass := generationErr == nil && after.SpeedPercent == scenario.speed
					if scenario.variation >= 0 {
						pass = pass && after.Gesture.VariationPercent == scenario.variation
					}
					if scenario.min >= 0 {
						pass = pass && after.MinPercent == scenario.min && after.MaxPercent == 100
					}
					if scenario.paused || index == 0 || index == 5 || index == 7 {
						pass = pass && result.Response.Motion == nil
					}
					if index == 1 {
						pass = pass && result.Response.Motion != nil && result.Response.Motion.Action == MotionActionStart
					}
					if index == 4 {
						pass = pass && after.Gesture.ReboundCount == 0 && after.Gesture.InertiaPercent == 70
					}
					errorText := ""
					if generationErr != nil {
						errorText = generationErr.Error()
					}
					row := map[string]any{"model": model, "method": "creative_v2", "variant": variant, "message": scenario.message, "reasoning": reasoning, "schema_guided": schema, "valid": generationErr == nil, "intent_pass": pass, "raw": result.Raw, "error": errorText, "before": score, "after": after, "limits": limits, "elapsed_ms": time.Since(started).Milliseconds(), "provider_calls": provider.calls, "prompt_bytes": len(provider.request.Messages[0].Content), "progress": provider.progress}
					rows = append(rows, row)
					writeReportProbe(t, "-actions", rows)
					t.Logf("%s schema=%t reasoning=%s case=%d valid=%t intent=%t ms=%v", variant, schema, reasoning, index, generationErr == nil, pass, row["elapsed_ms"])
				}
			}
		}
	}
}

func TestReportIssuesLiveCheckIns(t *testing.T) {
	models := strings.Split(os.Getenv("MAGICHANDY_REPORT_ISSUES_MODEL"), "|")
	if models[0] == "" {
		t.Skip("set MAGICHANDY_REPORT_ISSUES_MODEL")
	}
	rows := []map[string]any{}
	for _, model := range models {
		native, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		for _, guarded := range []bool{false, true} {
			if os.Getenv("MAGICHANDY_REPORT_ISSUES_QUIET") != "" && !guarded {
				continue
			}
			for _, scenario := range []string{"idle", "requested_updates", "conversation"} {
				history := []llm.Message{{Role: "user", Content: "Keep the session relaxed. Brief factual check-ins are welcome."}}
				voice := VoiceUtility
				if scenario == "requested_updates" {
					history[0].Content = "Please tell me when the speed changes, and when the travel direction changes. No need to report unchanged motion."
				}
				if scenario == "conversation" {
					voice = VoiceWarm
					history[0].Content = "The rain finally stopped and my garden smells wonderful. Keep me company and take the lead in our conversation."
				}
				last := ""
				provider := &reportProbeProvider{Provider: native, schema: true, legacySpeech: !guarded}
				service := AutopilotService{Provider: provider, Model: model, MaxTokens: 512, ReasoningMode: "off", Temperature: 0.55, Capabilities: Capabilities{Voice: voice}}
				for index := range 18 {
					context := AutopilotContext{SegmentIndex: index + 1, CurrentSpeed: 25, SpeedMinPercent: 10, SpeedMaxPercent: 43, LastSay: last, MotionMode: MotionModeCreativeV2, SessionTracking: true, SessionSeconds: (index + 1) * 40}
					age := (index + 1) * 40
					if index >= 9 && scenario == "requested_updates" {
						age = (index - 9) * 40
					}
					if index >= 12 && scenario == "conversation" {
						age = (index - 12) * 40
					}
					context.LastHumanSecondsAgo = &age
					context.SecondsAtCurrentSpeed, context.SecondsAtCurrentPhrase = (index+1)*40, (index+1)*40
					if index >= 9 {
						context.SecondsAtCurrentSpeed = (index - 9) * 40
					}
					if index >= 12 {
						context.SecondsAtCurrentPhrase = (index - 12) * 40
					}
					score := FreshCreativeV2Score(25)
					score.Seed = 1
					context.CurrentFlow = &score
					if index >= 9 {
						context.CurrentSpeed = 30
						score.SpeedPercent = 30
					}
					if index >= 12 {
						score.Gesture.FasterDirection, score.Gesture.ContrastPercent = "tip", 40
					}
					if index == 9 && scenario == "requested_updates" {
						history = append(history, llm.Message{Role: "user", Content: "I've just raised the pace to 30%. Please acknowledge the change briefly."})
					}
					if index == 12 && scenario == "conversation" {
						history = append(history, llm.Message{Role: "user", Content: "I think I'll plant some lavender next. What would you put beside it?"})
					}
					started := time.Now()
					message := AutopilotSpeechMessage(context)
					if !guarded {
						message = reportBaselineSpeechMessage(context)
					}
					response, err := service.Complete(t.Context(), AutopilotKindSpeech, Request{Message: message, History: history})
					duplicate := false
					for _, entry := range history {
						if entry.Role == "assistant" && entry.Content == response.Reply {
							duplicate = true
						}
					}
					published := err == nil && response.Reply != ""
					if published {
						history = append(history, llm.Message{Role: "assistant", Content: response.Reply})
						last = response.Reply
					}
					errorText := ""
					if err != nil {
						errorText = err.Error()
					}
					rows = append(rows, map[string]any{"model": model, "guarded": guarded, "scenario": scenario, "method": "creative_v2", "message": fmt.Sprintf("%s candidate=%t turn=%d", scenario, guarded, index+1), "turn": index + 1, "reply": response.Reply, "raw": provider.lastRaw, "valid": err == nil, "after": score, "limits": config.DefaultSettings().Motion, "duplicate": duplicate, "quiet": err == nil && response.Reply == "", "published": published, "elapsed_ms": time.Since(started).Milliseconds(), "error": errorText, "provider_calls": provider.calls, "provider_request": provider.request})
					writeReportProbe(t, "-checkins", rows)
					t.Logf("model=%s guarded=%t turn=%d duplicate=%t published=%t", model, guarded, index+1, duplicate, published)
				}
			}
		}
	}
}

// Baseline is the pre-change scheduled message, using the same factual state.
func reportBaselineSpeechMessage(context AutopilotContext) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Autopilot spoken check-in after motion decision %d.\n", context.SegmentIndex)
	if say := strings.TrimSpace(context.LastSay); say != "" {
		fmt.Fprintf(&builder, "The last autonomous line was: %q. Continue the same conversation naturally, without repeating this line or restarting the session.\n", say)
	}
	writeAutopilotSpeechFacts(&builder, context)
	if context.SessionTracking && context.SessionSeconds > 0 {
		fmt.Fprintf(&builder, "This Autopilot session has been running for %s.\n", formatSessionSpan(context.SessionSeconds))
	}
	builder.WriteString("Write one short line in the selected voice that fits the ongoing conversation (under 150 characters and no question that demands an answer). Let the previous exchange lead into this one; a new setting or event is not needed for every message. Do not invent a scene when no scene has been established. Ground any motion description in one specific current fact, such as working region, changing stroke length, directional timing or pace variation, instead of repeatedly asserting smoothness or steadiness. Describe commanded motion without claiming sensed feedback or force.\n")
	builder.WriteString(autopilotSpeechTimingInstruction)
	return builder.String()
}
