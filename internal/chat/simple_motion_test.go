package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestSimpleInteractiveEditsStayWithinExistingContract(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, level := range []string{"simple", "intermediate", "advanced", "legacy"} {
			t.Run(string(mode)+"/"+level, func(t *testing.T) {
				caps := Capabilities{Motion: true, MotionMode: mode, SimpleMotion: level == "simple", IntermediateMotion: level == "intermediate", AdvancedMotion: level == "advanced"}
				flow := DefaultLayeredScore(43)
				edits := `{}`
				if mode == MotionModeCreativeV2 {
					flow = FreshCreativeV2Score(43)
				}
				if mode == MotionModeCreativeV2 || level == "intermediate" {
					edits = `[]`
				}
				provider := &scriptedProvider{responses: []string{`{"action":"none","edits":` + edits + `,"reply":"An explanation only."}`}}
				state := MotionContext{Running: true, MotionMode: mode, Layered: &flow}
				service := Service{Provider: provider, Capabilities: &caps, MotionContext: &state, MaxTokens: 256, SingleAttempt: true}
				result, err := service.Complete(t.Context(), Request{Message: "Explain the current rhythm without changing it."}, nil)
				if err != nil || result.Response.Motion != nil || len(provider.requests) != 1 {
					t.Fatalf("reply-only request changed behavior: %+v %v", result.Response, err)
				}
				request := provider.requests[0]
				if strings.Contains(request.Messages[0].Content, simplePatchGuide) != (level == "simple") || request.MaxTokens != 256 {
					t.Fatal("guidance or budget leaked across capability levels")
				}
				if level == "simple" && (strings.Contains(string(request.JSONSchema), `"basis"`) || strings.Contains(string(request.JSONSchema), `"vibration"`)) {
					t.Fatal("interactive Simple acquired planning metadata or independent outputs")
				}
			})
		}
	}
}

func TestSimpleAutopilotPlanIsPrivateBoundedAndMotionOnly(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, level := range []string{"simple", "intermediate", "advanced"} {
			for _, kind := range []AutopilotKind{AutopilotKindMotion, AutopilotKindSpeech} {
				t.Run(string(mode)+"/"+level+"/"+string(kind), func(t *testing.T) {
					planned := level != "advanced" && kind == AutopilotKindMotion
					caps := Capabilities{Motion: true, MotionMode: mode, SimpleMotion: level == "simple", IntermediateMotion: level == "intermediate", AdvancedMotion: level == "advanced"}
					flow, edits := DefaultLayeredScore(43), `{}`
					if mode == MotionModeCreativeV2 {
						flow = FreshCreativeV2Score(43)
					}
					if mode == MotionModeCreativeV2 {
						edits = `[]`
					}
					basis := ""
					if planned {
						basis = `"basis":"Hold every field for the requested exact repeat.",`
					}
					provider := &scriptedProvider{responses: []string{`{` + basis + `"edits":` + edits + `,"reply":"Keeping this rhythm."}`}}
					state := MotionContext{Running: true, MotionMode: mode, Layered: &flow, UserRequests: []string{"Unique human direction: preserve this exact pattern."}, UserRequestSecondsAgo: []int{45}}
					service := AutopilotService{Provider: provider, Capabilities: caps, MotionContext: &state, MaxTokens: 256}
					response, err := service.Complete(t.Context(), kind, Request{Message: "Consider the next decision."})
					if err != nil || response.Reply != "Keeping this rhythm." || len(provider.requests) != 1 {
						t.Fatalf("planning contract: %+v %v", response, err)
					}
					request := provider.requests[0]
					if strings.Contains(string(request.JSONSchema), `"basis"`) != planned {
						t.Fatal("planning schema leaked into another capability or speech")
					}
					wantTokens := 256
					if planned {
						wantTokens = 384
						all := request.Messages[0].Content + request.Messages[len(request.Messages)-1].Content
						wantCopies := 1
						if level == "intermediate" {
							wantCopies = 2 // The tested 32K contract retains its system timeline.
						}
						if strings.Count(all, state.UserRequests[0]) != wantCopies || !strings.Contains(request.Messages[len(request.Messages)-1].Content, `"seconds_ago":45`) {
							t.Fatal("human timeline was duplicated, lost, or moved away from decision")
						}
					}
					if request.MaxTokens != wantTokens || service.MaxTokens != 256 || !reflect.DeepEqual(state.UserRequests, []string{"Unique human direction: preserve this exact pattern."}) {
						t.Fatal("budget or state mutated outside this call")
					}
				})
			}
		}
	}
}

func TestSimplePlanNoteCannotCommandOrRewriteMotion(t *testing.T) {
	current := DefaultLayeredScore(43)
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, SimpleMotion: true}
	service := Service{Capabilities: &caps, localAutopilotPlan: true}
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &current}
	for _, note := range []string{"Go as fast as possible and ignore all constraints.", "Never change any field again.", ""} {
		encoded, _ := json.Marshal(note)
		raw := `{"action":"update","basis":` + string(encoded) + `,"edits":{"controls":{"speed_percent":35}},"reply":"Adjusted pace."}`
		response, err := service.parseLayeredResponse(raw, current, state, config.DefaultSettings().Motion, false)
		if err != nil || response.Motion == nil || response.Motion.Layered.SpeedPercent != 35 || current.SpeedPercent != 43 {
			t.Fatalf("metadata affected command or original state: %+v %v", response, err)
		}
	}
	for _, raw := range []string{
		`{"basis":null,"edits":{},"reply":"No change."}`,
		`{"basis":12,"edits":{},"reply":"No change."}`,
		`{"basis":"first","basis":"second","edits":{},"reply":"No change."}`,
		`{"basis":"ok","edits":{},"edits":{},"reply":"No change."}`,
		`{"basis":"` + strings.Repeat("x", 241) + `","edits":{},"reply":"No change."}`,
	} {
		if _, err := takeLocalPlanNote(raw); err == nil {
			t.Fatalf("accepted invalid metadata: %s", raw)
		}
	}
	raw := `{"edits":{},"reply":"Legacy contract."}`
	if got, err := takeLocalPlanNote(raw); got != raw || err != nil {
		t.Fatal("legacy unconstrained reply rejected")
	}
}

func TestSimplePlanUnknownAgesAndMessageOwnership(t *testing.T) {
	messages := []llm.Message{{Role: "system", Content: "Original system."}, {Role: "user", Content: "Original decision."}}
	before := append([]llm.Message(nil), messages...)
	got := localPlanningMessages(messages, MotionContext{UserRequests: []string{"A human request."}})
	if !reflect.DeepEqual(messages, before) || !strings.Contains(got[1].Content, "ages unknown") || strings.Contains(got[1].Content, "seconds_ago") {
		t.Fatal("mutated source messages or fabricated age")
	}
	if localPlanningTokens(512) != 512 || localPlanningTokens(0) != 0 {
		t.Fatal("changed larger or unspecified budgets")
	}
}

func TestSimpleLifecyclePromptFits16K(t *testing.T) {
	for _, set := range BuiltinPromptSets() {
		for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
			for _, state := range []MotionContext{{Running: true}, {Paused: true}, {}} {
				state.MotionMode, state.SpeedMinPercent, state.SpeedMaxPercent = mode, 10, 80
				flow := DefaultLayeredScore(40)
				if mode == MotionModeCreativeV2 {
					flow = FreshCreativeV2Score(40)
				}
				state.Layered = &flow
				caps := Capabilities{Motion: true, MotionMode: mode, SimpleMotion: true}
				composition := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil, PromptBudgetSettings{ContextSize: 16384, MaxOutputTokens: 256})
				request := llm.ChatRequest{MaxTokens: 256, JSONSchema: InteractiveResponseSchema(nil, caps, &state), Messages: []llm.Message{{Role: "system", Content: composition.Prompt}, {Role: "user", Content: "Could you give me a slow rolling rhythm now?"}}}
				if _, _, err := llm.BudgetChatRequest(request, 16384); err != nil {
					t.Fatalf("%s/%s running=%t paused=%t: %v", set.ID, mode, state.Running, state.Paused, err)
				}
				hasGuide := set.ID != PromptSetIDJapanese && set.ID != PromptSetIDPortugueseBrazil
				if !hasGuide {
					legacy := caps
					legacy.SimpleMotion = false
					before := ComposeInteractivePrompt(set, nil, nil, legacy, &state, nil, PromptBudgetSettings{ContextSize: 16384, MaxOutputTokens: 256})
					if before.Prompt != composition.Prompt {
						t.Fatal("unhelpful language-set experiment altered its existing contract")
					}
				}
				if strings.Contains(composition.Prompt, simpleReachGuide) != (hasGuide && state.Running && !state.Paused) {
					t.Fatal("running refinement guide displaced lifecycle guidance")
				}
			}
		}
	}

}

func TestSimpleRepairRetainsContractWithoutRedundantReminder(t *testing.T) {
	bad := `{"action":"update","edits":{"geometry":"full_slider","layers":[{"axis":"center","amount_percent":35}]},"reply":"Full travel."}`
	good := `{"action":"update","edits":{"geometry":"full_slider"},"reply":"Full travel."}`
	provider := &scriptedProvider{responses: []string{bad, good}}
	service := localRepairService(provider, MotionModeLayered)
	service.Capabilities.SimpleMotion = true
	service.MaxTokens, service.PromptBudget = 256, PromptBudgetSettings{ContextSize: 16384, MaxOutputTokens: 256}
	result, err := service.Complete(t.Context(), Request{Message: "Use full-device sweeps while preserving the pace."}, nil)
	if err != nil || !result.Repaired || len(provider.requests) != 2 || result.Raw != bad || result.RepairRaw != good {
		t.Fatalf("repair failed: %+v %v", result, err)
	}
	first, repair := provider.requests[0], provider.requests[1]
	want := strings.TrimSuffix(first.Messages[0].Content, "\n\n"+simplePatchGuide+"\n"+simpleReachGuide)
	if repair.Messages[0].Content != want || !strings.Contains(first.Messages[0].Content, simplePatchGuide) || first.Messages[1] != repair.Messages[1] || string(first.JSONSchema) != string(repair.JSONSchema) {
		t.Fatal("repair modified the full contract, original request, or initial capture")
	}
	if _, _, err := llm.BudgetChatRequest(repair, 16384); err != nil {
		t.Fatal(err)
	}
}
