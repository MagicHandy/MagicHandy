package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestAdvancedLocalAutopilotUsesTimingAndRejectsStalePreparation(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{`{"edits":[{"evolve":true}],"next_check_seconds":150,"reply":"Developing slowly."}`}}
	s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(s.Close)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Provider, settings.LLM.LocalCapabilityLevel = config.LLMProviderOllama, config.ModelCapabilityAdvanced
		settings.LLM.LlamaCPPContextSize, settings.LLM.MaxOutputTokens = 65536, 1024
		settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		settings.Autopilot.AutoMotionTiming = true
		return settings
	})
	flow := chat.FreshCreativeV2Score(30)
	decision, err := s.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: &flow, CurrentSpeed: 30, SpeedMinPercent: 10, SpeedMaxPercent: 80, MotionChangeLevel: 4})
	if err != nil || decision.NextCheck != 150*time.Second || decision.Segment.Flow == nil || !decision.Abstain {
		t.Fatalf("advanced timing: %+v %v", decision, err)
	}
	if len(provider.requests) != 1 || !strings.Contains(string(provider.requests[0].JSONSchema), "next_check_seconds") {
		t.Fatal("timing not in request")
	}
	settings, _ := s.store.Snapshot()
	planning, _ := settings.LLM.PlanningSettings()
	prepared, err := s.prepareLLMProvider(planning)
	if err != nil {
		t.Fatal(err)
	}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.LocalCapabilityLevel = config.ModelCapabilitySimple
		return settings
	})
	_, err = prepared.StreamChat(t.Context(), llm.ChatRequest{}, nil)
	if !errors.Is(err, context.Canceled) || len(provider.requests) != 1 {
		t.Fatalf("downgraded request ran: %v", err)
	}
}

func TestAdvancedLocalIndependentVibrationUsesSameArmingAndStopGates(t *testing.T) {
	s := independentVibrationServer(t) // in-process fake Intiface, never hardware
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.ConversationConnectionID = "local"
		settings.LLM.LocalCapabilityLevel = config.ModelCapabilityAdvanced
		settings.LLM.LlamaCPPContextSize, settings.LLM.MaxOutputTokens = 65536, 1024
		return settings
	})
	spec := motion.VibrationSpec{Mode: "wave", MinPercent: 20, MaxPercent: 50, PeriodSeconds: 6}
	settings, _ := s.store.Snapshot()
	if !s.chatOutputContext(settings.LLM.ConversationSettings()).Vibrator.Independent {
		t.Fatal("local advanced output missing")
	}
	result, err := s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &spec})
	if err != nil || !result.Applied || !result.Engine.Target.VibrationOnly {
		t.Fatalf("independent local start: %+v %v", result, err)
	}
	if _, err := s.emergencyStop(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.validateChatVibration(&spec); err == nil {
		t.Fatal("advanced model bypassed zero intensity after Stop")
	}
}

func TestLocalTierReachesChatAndPlanningCapabilities(t *testing.T) {
	for _, level := range config.ModelCapabilityLevels() {
		t.Run(level, func(t *testing.T) {
			settings := config.DefaultSettings()
			settings.LLM.Provider = config.LLMProviderOllama
			settings.LLM.LocalCapabilityLevel = level
			settings.LLM.LlamaCPPContextSize = config.MinimumLocalContext(level)
			settings.LLM.MaxOutputTokens = config.MinimumLocalOutput(level)
			settings.Autopilot.AutoMotionTiming, settings.Autopilot.PrepareNextDecision = true, true
			caps := chatCapabilities(settings.LLM, nil)
			advanced := level == config.ModelCapabilityAdvanced
			if caps.AdvancedMotion != advanced || caps.IntermediateMotion != (level == config.ModelCapabilityIntermediate) || caps.PreserveConversationText != advanced || autoMotionTiming(chat.AutopilotKindMotion, settings) != advanced || prepareAutopilotDecisions(settings) != advanced {
				t.Fatalf("capabilities not applied: %+v", caps)
			}
			if autoMotionTiming(chat.AutopilotKindSpeech, settings) {
				t.Fatal("speech acquired timing")
			}
			budget := chatPromptBudget(settings.LLM)
			want := config.MinimumLocalContext(level)
			if level == config.ModelCapabilitySimple {
				want = 0
			}
			if budget.ContextSize != want {
				t.Fatalf("context=%d want=%d", budget.ContextSize, want)
			}
			downgrade := settings.LLM
			downgrade.LocalCapabilityLevel = config.ModelCapabilitySimple
			if level != config.ModelCapabilitySimple && !llmRuntimeSettingsChanged(settings.LLM, downgrade) {
				t.Fatal("tier change reused stale provider")
			}
		})
	}
}

func TestAdvancedLocalResponseRepairAndDowngradeRetirement(t *testing.T) {
	for _, automatic := range []bool{true, false} {
		for _, retire := range []bool{false, true} {
			if automatic && retire {
				continue
			}
			t.Run(fmt.Sprintf("auto=%t/retire=%t", automatic, retire), func(t *testing.T) {
				fake := transport.NewFake()
				provider := &scriptedLLMProvider{responses: []string{`{"reply":"Invalid","unknown":true}`, `{"reply":"Corrected reply."}`}}
				s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
				t.Cleanup(s.Close)
				saveSettings(t, s.store, func(settings config.Settings) config.Settings {
					settings.LLM.Provider, settings.LLM.LocalCapabilityLevel = config.LLMProviderOllama, config.ModelCapabilityAdvanced
					settings.LLM.LlamaCPPContextSize, settings.LLM.MaxOutputTokens = 65536, 1024
					settings.LLM.MotionGenerationMode = config.LLMMotionModeOff
					settings.LLM.HostedResponseRepairEnabled = &automatic
					return settings
				})
				stream := postChatStream(t, s, `{"message":"Only acknowledge this text. No motion."}`)
				if !automatic {
					session, id := repairOffer(t, s, stream)
					if retire {
						saveSettings(t, s.store, func(settings config.Settings) config.Settings {
							settings.LLM.LocalCapabilityLevel = config.ModelCapabilitySimple
							return settings
						})
					}
					reply := postRepair(s, session, id)
					stream = reply.Body.String()
				}
				if retire {
					if len(provider.requests) != 1 || strings.Contains(stream, "Corrected reply.") {
						t.Fatalf("stale repair ran: %s", stream)
					}
				} else if len(provider.requests) != 2 || !strings.Contains(stream, "Corrected reply.") || !strings.Contains(stream, `"repaired":true`) {
					t.Fatalf("advanced local repair failed: calls=%d %s", len(provider.requests), stream)
				}
				if len(fake.Commands()) != 0 {
					t.Fatal("text-only repair moved hardware")
				}
			})
		}
	}
}

func TestAdvancedLocalHistoryUsesBoundedExternalWindow(t *testing.T) {
	s := newTestServerWithRuntime(t, Runtime{})
	t.Cleanup(s.Close)
	for i := 0; i < 90; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		if _, err := s.chatLog.Append(role, fmt.Sprintf("Preserve this dialogue %d", i), "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	session, _ := s.chatLog.ActiveSessionID()
	for _, level := range config.ModelCapabilityLevels() {
		settings := config.DefaultSettings().LLM
		settings.LocalCapabilityLevel = level
		settings.LlamaCPPContextSize = config.MinimumLocalContext(level)
		context, err := s.loadInteractiveChatPromptContext(t.Context(), session, settings)
		if err != nil {
			t.Fatal(err)
		}
		want := chat.PromptHistoryLimit
		if level == config.ModelCapabilityAdvanced {
			want = 64
		}
		if len(context.History) != want {
			t.Fatalf("%s history=%d want=%d", level, len(context.History), want)
		}
	}
}

func TestLocalCapabilityOutputFactsReachProductionTurns(t *testing.T) {
	s := independentVibrationServer(t)
	for _, level := range config.ModelCapabilityLevels() {
		saveSettings(t, s.store, func(settings config.Settings) config.Settings {
			settings.LLM.ConversationConnectionID = "local"
			settings.LLM.LocalCapabilityLevel = level
			settings.LLM.LlamaCPPContextSize = config.MinimumLocalContext(level)
			settings.LLM.MaxOutputTokens = max(512, config.MinimumLocalOutput(level))
			return settings
		})
		settings, _ := s.store.Snapshot()
		settings.LLM = settings.LLM.ConversationSettings()
		state := s.chatTurnMotion(settings, nil, "")
		if level == config.ModelCapabilitySimple {
			if state.Outputs != nil {
				t.Fatal("simple local contract acquired output context")
			}
			continue
		}
		if state.Outputs == nil || state.Outputs.Strokers != 1 || state.Outputs.Vibrator == nil ||
			!state.Outputs.Vibrator.Ready || !state.Outputs.Vibrator.Armed ||
			state.Outputs.Vibrator.Independent != (level == config.ModelCapabilityAdvanced) {
			t.Fatalf("%s live context lost output facts: %+v", level, state.Outputs)
		}
	}
}

func TestIntermediateUnarmedVibrationFactsReachChatRequest(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{`{"action":"none","edits":[],"reply":"Connect the vibrator and set its intensity first."}`}}
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(s.Close)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Device, _ = settings.Device.ChangeConnection(config.DeviceVibrator, "add")
		settings.Device = settings.Device.WithPlaybackDevices([]string{config.DeviceVibrator})
		settings.LLM.Provider = config.LLMProviderOllama
		settings.LLM.LocalCapabilityLevel = config.ModelCapabilityIntermediate
		settings.LLM.LlamaCPPContextSize, settings.LLM.MaxOutputTokens = 32768, 512
		settings.LLM.MotionGenerationMode = config.LLMMotionModeLayered
		return settings
	})
	stream := postChatStream(t, s, `{"message":"Start vibration now."}`)
	if len(provider.requests) != 1 || !strings.Contains(stream, "Connect the vibrator") || strings.Contains(stream, "event: error") {
		t.Fatalf("reply-only production turn: %s", stream)
	}
	var prompt strings.Builder
	for _, message := range provider.requests[0].Messages {
		prompt.WriteString(message.Content)
	}
	if !strings.Contains(prompt.String(), `"connected_and_selected":false`) ||
		!strings.Contains(prompt.String(), `"user_has_set_nonzero_intensity":false`) ||
		strings.Contains(string(provider.requests[0].JSONSchema), `"const":"start"`) || len(fake.Commands()) != 0 {
		t.Fatal("live chat omitted output readiness or retained start authority")
	}
}
