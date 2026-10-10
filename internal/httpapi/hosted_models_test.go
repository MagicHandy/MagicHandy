package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func hostedFixture(t *testing.T, reply func(http.ResponseWriter, *http.Request)) (*Server, *transport.Fake, config.ModelConnection) {
	t.Helper()
	endpoint := httptest.NewServer(http.HandlerFunc(reply))
	t.Cleanup(endpoint.Close)
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	connection := config.ModelConnection{ID: "hosted", Name: "Fixture", Provider: config.LLMProviderCompatible, BaseURL: endpoint.URL, Model: "fixture-model", NoAuthentication: true}.Normalize()
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Connections = []config.ModelConnection{connection}
		settings.LLM.ConversationConnectionID = connection.ID
		settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: "conversation", ContextPolicy: config.ContextConversation}
		settings.LLM.MotionGenerationMode = config.LLMMotionModeOff
		return settings
	})
	return s, fake, connection
}

func writeHostedFixtureReply(w http.ResponseWriter, raw string) {
	w.Header().Set("Content-Type", "text/event-stream")
	encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": raw}}}})
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", encoded)
}

func TestHostedOnlyChatPreservesCanonicalHistoryWithoutLocalRuntime(t *testing.T) {
	captured := make(chan []llm.Message, 3)
	var calls atomic.Int32
	s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected local/load endpoint %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		calls.Add(1)
		var request llm.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		captured <- request.Messages
		writeHostedFixtureReply(w, `{"reply":"Cloud reply."}`)
	})
	priorUser := "Consenting adults used NSFW wording: orgasm.\n" + strings.Repeat("x", 4300)
	priorAssistant := "Original reply.\n" + strings.Repeat("y", 4300)
	if _, err := s.chatLog.Append(chat.MessageRoleUser, priorUser, "seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.chatLog.Append(chat.MessageRoleAssistant, priorAssistant, ""); err != nil {
		t.Fatal(err)
	}
	stream := postChatStream(t, s, `{"message":"Continue with text only."}`)
	messages := <-captured
	if !strings.Contains(stream, `"reply":"Cloud reply."`) || strings.Contains(stream, "event: malformed") || strings.Contains(stream, "event: motion") || calls.Load() != 1 {
		t.Fatalf("hosted chat = %s, calls=%d", stream, calls.Load())
	}
	for _, original := range []llm.Message{{Role: "user", Content: priorUser}, {Role: "assistant", Content: priorAssistant}} {
		if !slices.Contains(messages, original) {
			t.Fatalf("canonical %s was wrapped or shortened", original.Role)
		}
	}
	if len(fake.Commands()) != 0 || s.managedLLM.Snapshot().Runtime.Installed {
		t.Fatal("hosted-only text chat required a local runtime or dispatched motion")
	}
}

func TestHostedSetupRequiresRealCompletionAndRejectsChangedCredentials(t *testing.T) {
	s, fake, connection := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) { writeHostedFixtureReply(w, `{"ready":true}`) })
	settings, _ := s.store.Snapshot()
	if err := s.validateSetupConnections(settings); err == nil {
		t.Fatal("typed model ID admitted untested setup")
	}
	body, _ := json.Marshal(map[string]any{"connection": connection})
	r := withController(httptest.NewRequest(http.MethodPost, "/api/llm/connections/test", strings.NewReader(string(body))))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ready":true`) {
		t.Fatalf("connection test = %d, %s", w.Code, w.Body.String())
	}
	if err := s.validateSetupConnections(settings); err != nil {
		t.Fatalf("actual completion was lost on setup save: %v", err)
	}
	if len(fake.Commands()) != 0 {
		t.Fatal("text connection test dispatched motion")
	}
	if err := s.cloudPlanning.auth.SetAPIKey(t.Context(), "different-credential-generation"); err != nil {
		t.Fatal(err)
	}
	if err := s.validateSetupConnections(settings); err == nil {
		t.Fatal("old completion survived a credential generation change")
	}
}

func TestHostedDraftKeyLookupIsExactRedactedAndDoesNotMutate(t *testing.T) {
	var calls atomic.Int32
	s, fake, connection := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(500) })
	connection.NoAuthentication = false
	const secret = "private-draft-key"
	if err := s.cloudPlanning.auth.SetConnectionKey(t.Context(), connection, secret); err != nil {
		t.Fatal(err)
	}
	generation := s.cloudPlanning.auth.Generation()
	for _, changed := range []bool{false, true} {
		candidate := connection
		if changed {
			candidate.BaseURL += "/other"
		}
		body, _ := json.Marshal(map[string]any{"connection": candidate})
		request := withController(httptest.NewRequest(http.MethodPost, "/api/llm/connections/key", strings.NewReader(string(body))))
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != 200 || strings.Contains(response.Body.String(), secret) {
			t.Fatalf("draft binding lookup failed or exposed a key: %d", response.Code)
		}
		var result struct {
			KeySet bool `json:"key_set"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.KeySet == changed {
			t.Fatalf("wrong endpoint inherited draft key: %s", response.Body.String())
		}
	}
	if s.cloudPlanning.auth.Generation() != generation || !s.cloudPlanning.auth.ConnectionKeySet(t.Context(), connection) || calls.Load() != 0 || len(fake.Commands()) != 0 {
		t.Fatal("key lookup mutated credentials, generated text or dispatched motion")
	}
}

func TestHostedAutopilotHoldsRefusalWithoutLocalCallsOrDispatch(t *testing.T) {
	var calls atomic.Int32
	s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"refusal\":\"refused\"}}]}\n\ndata: [DONE]\n\n")
	})
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionGenerationMode = config.LLMMotionModePattern
		return settings
	})
	_, err := s.autopilotDecide(t.Context(), modes.DecisionInput{})
	var outcome interface{ HoldMotion() bool }
	if !errors.As(err, &outcome) || !outcome.HoldMotion() || calls.Load() != 1 || len(fake.Commands()) != 0 {
		t.Fatalf("refusal invoked fallback or dispatch: %v, calls=%d", err, calls.Load())
	}
}

func TestHostedUnstructuredRefusalAndMalformedOutputNeverRepairOrDispatch(t *testing.T) {
	for name, raw := range map[string]string{
		"plain refusal": "I cannot help with this request.",
		"malformed":     `{"reply":"unfinished`,
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeHostedFixtureReply(w, raw)
			})
			stream := postChatStream(t, s, `{"message":"Text only, please."}`)
			if calls.Load() != 1 || strings.Contains(stream, "event: reply") || strings.Contains(stream, "event: motion") || len(fake.Commands()) != 0 {
				t.Fatalf("hosted chat repaired or admitted invalid output: calls=%d, stream=%s", calls.Load(), stream)
			}
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.MotionGenerationMode = config.LLMMotionModePattern
				return settings
			})
			_, err := s.autopilotDecide(t.Context(), modes.DecisionInput{})
			var outcome *llm.CloudError
			if !errors.As(err, &outcome) || !outcome.HoldMotion() || calls.Load() != 2 || len(fake.Commands()) != 0 {
				t.Fatalf("hosted Autopilot repaired or fell back: err=%v, calls=%d", err, calls.Load())
			}
		})
	}
}

func TestTechnicalPlannerExcludesConversationAndCustomLabels(t *testing.T) {
	private := "private-conversation-persona-memory"
	definition := motion.DynamicDefinition{Anchors: []motion.DynamicAnchor{{Name: private, PositionPercent: 10}, {Name: private, PositionPercent: 90}}}
	input := modes.DecisionInput{CurrentDynamic: &definition, Style: private, MotionFeedback: private, LastSay: private, CurrentPatternID: motion.PatternID(private)}
	settings := config.DefaultSettings()
	evidence := technicalMotionEvidence(input, technicalStartingDecision(input, settings), settings)
	if strings.Contains(evidence, private) || !strings.Contains(evidence, `"position_percent":10`) {
		t.Fatalf("technical allowlist leaked labels or lost motion: %s", evidence)
	}
	if definition.Anchors[0].Name != private {
		t.Fatal("privacy serialization changed the engine's current target")
	}
}

// Technical context in a continuous mode is shown motion state only, yet plans
// with the same continuous Autopilot contract as the conversation policy,
// pace and outer reach included. A chat-set speed is marked without words.
func TestTechnicalAutopilotSharesMotionStateOnlyAndPlansPaceAndReach(t *testing.T) {
	private := "private-conversation-persona-memory"
	requests := make(chan []llm.Message, 1)
	s, fake, connection := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		requests <- request.Messages
		writeHostedFixtureReply(w, `{"edits":[{"speed_percent":55},{"range":{"min_percent":20,"max_percent":90}}],"reply":"Building up."}`)
	})
	session, err := s.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.chatLog.AppendTo(session, chat.MessageRoleUser, private, "test", nil); err != nil {
		t.Fatal(err)
	}
	// Long-term memories are the third thing the UI promises stays local.
	if _, err := s.personalization.memory.Add(private); err != nil {
		t.Fatal(err)
	}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 10, 80
		settings.LLM.ConversationConnectionID = "local"
		settings.LLM.PersonaDescription = private
		settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: "connection", ConnectionID: connection.ID, ContextPolicy: config.ContextTechnical}
		settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		return settings
	})
	flow := chat.FreshCreativeV2Score(30)
	decision, err := s.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: &flow, CurrentSpeed: 30, SpeedMinPercent: 10, SpeedMaxPercent: 80,
		RecentSpeeds: []modes.SpeedStep{{SpeedPercent: 40, SecondsAgo: 90}, {SpeedPercent: 30, SecondsAgo: 40, Interactive: true}}})
	if err != nil || decision.Hold || !decision.Abstain || decision.Segment.SpeedPercent != 55 || decision.Segment.Flow == nil || decision.Segment.Flow.MinPercent != 20 || decision.Segment.Flow.MaxPercent != 90 {
		t.Fatalf("technical plan was not accepted: %+v %v", decision, err)
	}
	messages := <-requests
	var sent strings.Builder
	for _, message := range messages {
		sent.WriteString(message.Content)
	}
	if len(messages) != 2 || strings.Contains(sent.String(), private) || !strings.Contains(sent.String(), "motion state only") || !strings.Contains(sent.String(), "40%, 30% (set in chat)") {
		t.Fatalf("technical planning shared personal context or lost motion state:\n%s", sent.String())
	}
	if len(fake.Commands()) != 0 {
		t.Fatal("planning dispatched motion")
	}
}

func TestHostedLanesRemainIndependentAndInvalidateTogether(t *testing.T) {
	s := newTestServer(t)
	first := config.DefaultSettings().LLM.WithConnection(config.ModelConnection{ID: "first", Name: "First", Provider: config.LLMProviderCompatible, BaseURL: "https://first.test/v1", Model: "model"})
	second := first.WithConnection(config.ModelConnection{ID: "second", Name: "Second", Provider: config.LLMProviderCompatible, BaseURL: "https://second.test/v1", Model: "model"})
	active, _, releaseFirst, err := s.acquireProviderRequest(t.Context(), llmRequestAutonomous, first)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	other, _, releaseSecond, err := s.acquireProviderRequest(ctx, llmRequestInteractive, second)
	if err != nil {
		t.Fatalf("independent endpoint queued behind another model: %v", err)
	}
	defer releaseSecond()
	if active.Err() != nil {
		t.Fatal("other connection preempted motion planning")
	}
	s.invalidateCloudPlanning()
	if active.Err() == nil || other.Err() == nil {
		t.Fatal("provider change did not cancel both lanes")
	}
}
