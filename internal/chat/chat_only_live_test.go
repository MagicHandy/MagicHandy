//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLiveVideoChatStillConverses(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := newLiveEvalProvider(llm.HTTPProviderOptions{BaseURL: liveEvalLlamaURL, Model: model, Timeout: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	for _, voice := range []VoiceLevel{VoiceUtility, VoiceWarm} {
		for _, holder := range []MotionHolder{MotionHolderVideoScript, MotionHolderVideoOff} {
			t.Run(string(voice)+"/"+string(holder), func(t *testing.T) {
				capabilities := Capabilities{Voice: voice, MotionHolder: holder}
				service := Service{Provider: provider, Model: model, Capabilities: &capabilities, MaxTokens: 256, ReasoningMode: "off"}
				result, err := service.Complete(t.Context(), Request{Message: "Welcome me back in one short sentence, and mention the tea I'm making. We're just chatting."}, nil)
				t.Logf("raw=%s", result.Raw)
				if err != nil || result.Malformed || result.Repaired || result.SemanticFallback || result.Response.Motion != nil || result.Response.Reply == "" {
					t.Fatalf("ordinary conversation failed: %+v err=%v", result, err)
				}
				for _, term := range []string{"script", "motion source", "settings", "device"} {
					if strings.Contains(strings.ToLower(result.Response.Reply), term) {
						t.Errorf("ordinary greeting unexpectedly discusses %q", term)
					}
				}
			})
		}
	}
}

// Inject only an invalid timing enum. The real model must repair the actual
// Autopilot service request without changing the selected reply language.
type invalidSpeechTimingProvider struct {
	llm.Provider
	reply string
	calls int
}

func (p *invalidSpeechTimingProvider) StreamChat(ctx context.Context, request llm.ChatRequest, onDelta func(string) error) (string, error) {
	p.calls++
	if p.calls == 1 {
		raw, _ := json.Marshal(map[string]string{"reply": p.reply, "next": "invalid"})
		return string(raw), nil
	}
	return p.Provider.StreamChat(ctx, request, onDelta)
}

func TestLiveAutopilotSpeechRepairLanguage(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := newLiveEvalProvider(llm.HTTPProviderOptions{BaseURL: liveEvalLlamaURL, Model: model, Timeout: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ id, reply string }{
		{DefaultPromptSetID, "Hello, I'm here with you."},
		{PromptSetIDSpanish, "Hola, estoy aquí contigo."},
		{PromptSetIDPortugueseBrazil, "Olá, estou aqui com você."},
		{PromptSetIDSimplifiedChinese, "你好，我就在这里陪着你。"},
		{PromptSetIDJapanese, "こんにちは、ここでお待ちしています。"},
	} {
		t.Run(test.id, func(t *testing.T) {
			injected := &invalidSpeechTimingProvider{Provider: provider, reply: test.reply}
			prompt, _ := BuiltinPromptSetByID(test.id)
			service := AutopilotService{Provider: injected, Model: model, Prompt: prompt, Capabilities: Capabilities{Voice: VoiceUtility}, MaxTokens: 256, ReasoningMode: "off"}
			response, err := service.Complete(t.Context(), AutopilotKindSpeech, Request{Message: "Offer a brief greeting in the selected language."})
			if err != nil || injected.calls != 2 || response.Motion != nil {
				t.Fatalf("speech repair: %+v calls=%d err=%v", response, injected.calls, err)
			}
			t.Logf("repaired reply=%s", response.Reply)
			assertVideoReplyLanguage(t, promptLocaleForID(test.id), response.Reply)
		})
	}
}

// Uses the production Service with a real model, but no engine or transport.
// Requests are deliberately non-graphic; selected adult registers have separate
// composition coverage and are not an assertion about generated prose quality.
func TestLiveVideoChatCapabilityAndLanguage(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := newLiveEvalProvider(llm.HTTPProviderOptions{BaseURL: liveEvalLlamaURL, Model: model, Timeout: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		promptID string
		request  string
	}{
		{DefaultPromptSetID, "Make the device move more slowly, please."},
		{PromptSetIDSpanish, "Haz que el dispositivo se mueva más despacio, por favor."},
		{PromptSetIDPortugueseBrazil, "Faça o dispositivo se mover mais devagar, por favor."},
		{PromptSetIDSimplifiedChinese, "请让设备移动得慢一点。"},
		{PromptSetIDJapanese, "デバイスの動きをもう少し遅くしてください。"},
	}
	for _, test := range cases {
		for _, holder := range []MotionHolder{MotionHolderVideoScript, MotionHolderVideoOff} {
			t.Run(test.promptID+"/"+string(holder), func(t *testing.T) {
				prompt, _ := BuiltinPromptSetByID(test.promptID)
				capabilities := Capabilities{Voice: VoiceUtility, MotionHolder: holder}
				service := Service{Provider: provider, Model: model, Prompt: prompt, Capabilities: &capabilities, MaxTokens: 256, ReasoningMode: "off"}
				result, err := service.Complete(t.Context(), Request{Message: test.request, History: []llm.Message{
					{Role: "user", Content: "The video is paused. We are only talking."},
					{Role: "assistant", Content: `{"reply":"Understood.","motion":{"action":"none"}}`},
				}}, nil)
				t.Logf("raw=%s", result.Raw)
				if err != nil || result.Malformed || result.Repaired || result.SemanticFallback || result.Response.Motion != nil {
					t.Fatalf("chat-only failed without a clean first response: %+v err=%v", result, err)
				}
				assertVideoReplyLanguage(t, promptLocaleForID(test.promptID), result.Response.Reply)
				if !strings.Contains(strings.ToLower(result.Response.Reply), "chat") &&
					!strings.Contains(result.Response.Reply, "チャット") && !strings.Contains(result.Response.Reply, "聊天") {
					t.Error("capability explanation did not name the Chat source control")
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal([]byte(result.Raw), &fields); err != nil || len(fields) != 1 || fields["reply"] == nil {
					t.Errorf("chat-only generated fields outside its contract: %s (err=%v)", result.Raw, err)
				}
			})
		}
	}
}

// The older conversation-language probe expects affectionate vocabulary.
// These probes cover capability explanations and neutral greetings instead.
func assertVideoReplyLanguage(t *testing.T, locale promptLocale, reply string) {
	t.Helper()
	var terms []string
	switch locale {
	case promptLocaleSpanish:
		terms = []string{"movimiento", "dispositivo", "fuente", "puedo", "puede", "guion", "seleccion", "cambiar", "hola", "contigo", "estoy"}
	case promptLocalePortugueseBrazil:
		terms = []string{"movimento", "dispositivo", "fonte", "posso", "pode", "roteiro", "selecione", "alterar", "olá", "você", "estou"}
	default:
		assertLiveReplyLanguage(t, locale, reply)
		return
	}
	score := 0
	for _, term := range terms {
		if strings.Contains(strings.ToLower(reply), term) {
			score++
		}
	}
	if score < 2 {
		t.Fatalf("capability reply needs a language review: %q", reply)
	}
}
