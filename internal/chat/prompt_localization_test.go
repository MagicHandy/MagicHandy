package chat

import (
	"strings"
	"testing"
)

type localizedPromptCase struct {
	name           string
	promptID       string
	identityMarker string
	profileMarker  string
	anatomyMarker  string
	recentMarker   string
	languageMarker string
	repairMarker   string
}

func TestBuiltinLocalizedPromptsLocalizeHumanProseAndPreserveProtocol(t *testing.T) {
	tests := []localizedPromptCase{
		{
			name:           "Spanish",
			promptID:       PromptSetIDSpanish,
			identityMarker: "IDENTIDAD DE RESPUESTA - PAREJA EXPLÍCITA:",
			profileMarker:  "PERFIL DEL CHAT:",
			anatomyMarker:  "Mi anatomía se describe como",
			recentMarker:   "LÍNEAS RECIENTES DE LA ASISTENTE",
			languageMarker: "IDIOMA FINAL:",
			repairMarker:   "debe permanecer en español",
		},
		{
			name:           "Brazilian Portuguese",
			promptID:       PromptSetIDPortugueseBrazil,
			identityMarker: "IDENTIDADE DA RESPOSTA - PARCEIRA EXPLÍCITA:",
			profileMarker:  "PERFIL DO CHAT:",
			anatomyMarker:  "Minha anatomia é descrita como",
			recentMarker:   "FALAS RECENTES DA ASSISTENTE",
			languageMarker: "IDIOMA FINAL:",
			repairMarker:   "deve permanecer em português do Brasil",
		},
		{
			name:           "Simplified Chinese",
			promptID:       PromptSetIDSimplifiedChinese,
			identityMarker: "回复身份 - 露骨伴侣：",
			profileMarker:  "聊天配置：",
			anatomyMarker:  "我的身体部位描述为",
			recentMarker:   "助手最近的回复",
			languageMarker: "最终语言：",
			repairMarker:   "必须保持为简体中文",
		},
		{
			name:           "Japanese",
			promptID:       PromptSetIDJapanese,
			identityMarker: "返答の役割 - 露骨なパートナー：",
			profileMarker:  "チャットプロフィール：",
			anatomyMarker:  "私の身体は",
			recentMarker:   "直近のアシスタント発言",
			languageMarker: "最終言語：",
			repairMarker:   "日本語のままにしてください",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertLocalizedPrompt(t, test)
		})
	}
}

func assertLocalizedPrompt(t *testing.T, test localizedPromptCase) {
	t.Helper()
	prompt, ok := BuiltinPromptSetByID(test.promptID)
	if !ok {
		t.Fatalf("missing built-in prompt set %q", test.promptID)
	}
	capabilities := FullCapabilities()
	capabilities.Voice = VoiceExplicit
	capabilities.MoodTracking = true
	context := ConversationContext{
		PersonaDescription:     `PERSONA "SENTINEL"`,
		UserAnatomy:            "custom",
		CustomAnatomy:          `CUSTOM "TERM"`,
		CurrentMood:            MoodCurious,
		RecentAssistantReplies: []string{`RECENT "LINE"`},
	}
	system := composeSystem(
		prompt,
		[]string{"MEMORY_SENTINEL"},
		[]PatternChoice{{ID: "steady_wave", Name: "Steady wave"}},
		capabilities,
		nil,
		&context,
	)

	for _, marker := range []string{
		test.identityMarker,
		test.profileMarker,
		test.anatomyMarker,
		test.recentMarker,
		test.languageMarker,
		`PERSONA \"SENTINEL\"`,
		`CUSTOM \"TERM\"`,
		`RECENT \"LINE\"`,
		"MEMORY_SENTINEL",
		`"action":"start"`,
		`"pattern_id"`,
		`"speed_percent"`,
		`top-level "new_mood"`,
	} {
		if !strings.Contains(system, marker) {
			t.Fatalf("localized prompt missing %q:\n%s", marker, system)
		}
	}
	if strings.Contains(system, "REPLY IDENTITY - EXPLICIT PARTNER:") {
		t.Fatal("localized prompt retained the English human-facing identity block")
	}
	voiceAt := strings.LastIndex(system, test.identityMarker)
	languageAt := strings.LastIndex(system, test.languageMarker)
	guardAt := strings.LastIndex(system, finalOutputGuardWithMood)
	if voiceAt == -1 || languageAt <= voiceAt || guardAt <= languageAt || !strings.HasSuffix(system, finalOutputGuardWithMood) {
		t.Fatalf("localized prompt terminal order voice=%d language=%d guard=%d", voiceAt, languageAt, guardAt)
	}

	repair := RepairPrompt(prompt, "PARSE_ERROR_SENTINEL")
	if !strings.Contains(repair, test.repairMarker) || !strings.Contains(repair, "PARSE_ERROR_SENTINEL") {
		t.Fatalf("repair prompt does not preserve %s: %s", test.name, repair)
	}
}

func TestCustomPromptLanguageIsNotForcedToEnglish(t *testing.T) {
	custom := PromptSet{ID: "custom-spanish", Name: "Custom", System: "Responde siempre en español."}
	system := ComposeSystem(custom, nil)
	if strings.Contains(system, "FINAL LANGUAGE:") || strings.Contains(system, "IDIOMA FINAL:") {
		t.Fatalf("custom prompt received a built-in language override:\n%s", system)
	}
	if !strings.HasSuffix(system, finalOutputGuard) {
		t.Fatal("custom prompt lost the terminal machine-readable output guard")
	}
}
