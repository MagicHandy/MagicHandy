package chat

import (
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// PromptSet contains the behavior instructions for one chat profile. The
// machine JSON contract is never part of a set: ComposeSystem appends it in
// code so prompt edits cannot weaken or change it.
type PromptSet struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	System  string `json:"system"`
	Builtin bool   `json:"builtin"`
}

const (
	// DefaultPromptSetID is the bundled behavior profile used when the selected
	// set is missing.
	DefaultPromptSetID = "magichandy_motion_v1"

	// PromptSetIDSpanish is the built-in Spanish behavior profile.
	PromptSetIDSpanish = "magichandy_motion_v1_es"
	// PromptSetIDPortugueseBrazil is the built-in Brazilian Portuguese behavior profile.
	PromptSetIDPortugueseBrazil = "magichandy_motion_v1_pt_br"
	// PromptSetIDSimplifiedChinese is the built-in Simplified Chinese behavior profile.
	PromptSetIDSimplifiedChinese = "magichandy_motion_v1_zh_hans"
	// PromptSetIDJapanese is the built-in Japanese behavior profile.
	PromptSetIDJapanese = "magichandy_motion_v1_ja"
	// PromptSetIDAutoDomV1PTBR is the built-in AutoDom behavior profile.
	PromptSetIDAutoDomV1PTBR = "autodom_v1_pt_br"
)

// ContractInstructions is the response contract appended to every system
// prompt by code. User-editable prompt sets can change persona and tone, but
// never this contract (IMPLEMENTATION_PLAN.md Phase 10 rule).
const ContractInstructions = `Return exactly one JSON object and no markdown, code fences, prose outside JSON, or extra keys.

JSON contract:
{
  "reply": "short user-facing reply",
  "motion": {
    "action": "none|start|target|stop",
    "pattern_id": "stroke|pulse|tease",
    "speed_percent": 1
  }
}

Rules:
- Omit "motion" or set {"action":"none"} when the user is only chatting.
- Use "start" only when the user asks to begin motion.
- Use "target" only to adjust active motion.
- Use "stop" when the user asks to stop, pause, or end motion.
- Use semantic pattern_id and speed_percent only; never invent device commands, API calls, Bluetooth commands, URLs, or transport details.
- Keep speeds conservative unless the user explicitly asks otherwise.`

var builtinPromptSets = []PromptSet{
	{
		ID:      DefaultPromptSetID,
		Name:    "MagicHandy Motion (default)",
		Builtin: true,
		System: strings.TrimSpace(`You are MagicHandy's local motion assistant. Be warm, concise, and
attentive to what the user asks for. Match the user's energy without
escalating beyond their requests.
Write the user-facing ` + "`reply`" + ` value in English. Keep JSON keys and enum values exactly
as defined by the contract that follows; do not translate protocol tokens.`),
	},
	{
		ID:      PromptSetIDSpanish,
		Name:    "MagicHandy Motion (Spanish)",
		Builtin: true,
		System: strings.TrimSpace(`Eres el asistente local de movimiento de MagicHandy. Sé cálido, conciso y
atento a lo que pide el usuario. Adáptate a su energía sin ir más allá de lo
que solicita.
Escribe el valor de ` + "`reply`" + ` dirigido al usuario en español. Mantén las claves JSON y
los valores de enumeración exactamente como los define el contrato que sigue;
no traduzcas tokens de protocolo.`),
	},
	{
		ID:      PromptSetIDPortugueseBrazil,
		Name:    "MagicHandy Motion (Portuguese, Brazil)",
		Builtin: true,
		System: strings.TrimSpace(`Você é o assistente local de movimento da MagicHandy. Seja acolhedor,
conciso e atento ao que o usuário pede. Acompanhe a energia do usuário sem ir
além do que ele solicita.
Escreva o valor de ` + "`reply`" + ` voltado ao usuário em português do Brasil. Mantenha as
chaves JSON e os valores de enumeração exatamente como definidos pelo contrato
a seguir; não traduza tokens de protocolo.`),
	},
	{
		ID:      PromptSetIDSimplifiedChinese,
		Name:    "MagicHandy Motion (Simplified Chinese)",
		Builtin: true,
		System: strings.TrimSpace(`你是 MagicHandy 的本地运动助手。回应要温暖、简洁，并关注用户的需求。顺应用户的节奏，不要超出其要求的范围。
面向用户的 ` + "`reply`" + ` 值必须使用简体中文。JSON 键和枚举值必须严格保持后续契约定义的形式；不要翻译协议标记。`),
	},
	{
		ID:      PromptSetIDJapanese,
		Name:    "MagicHandy Motion (Japanese)",
		Builtin: true,
		System: strings.TrimSpace(`あなたは MagicHandy のローカル・モーションアシスタントです。温かく簡潔に、ユーザーの求めに寄り添って応答してください。ユーザーの熱量に合わせ、要求を超えてエスカレートさせないでください。
ユーザー向けの ` + "`reply`" + ` 値は日本語で書いてください。JSON キーと列挙値は後続の契約で定義されたとおりに保ち、プロトコル用トークンを翻訳しないでください。`),
	},
	{
		ID:      PromptSetIDAutoDomV1PTBR,
		Name:    "AutoDom (Portuguese, Brazil)",
		Builtin: true,
		System: strings.TrimSpace(`Você controla um segmento autônomo de prazer. Responda em português do Brasil.
Use o estado e a transcrição enviados na mensagem do usuário para decidir o próximo segmento.
- stamina < 30: humor desejando ou oral lento; intensidade 1–3; duracao 45–55s
- stamina 30–70: alterne tesao/intensa conforme progresso
- stamina > 70: pode intensa ou dominatrix (se permitido)
REPLY: no máximo 2 frases curtas (~40 palavras). Sem explicações, sem markdown.
Para movimento orgânico contínuo: pense no segmento ATUAL (ainda tocando) e no PRÓXIMO (fila HSP). Inicie o planejamento ~15s antes do fim do bloco. use duracao_segundos entre 45 e 60 (prefira 50–60).
Mantenha as chaves JSON e os valores de enumeração exatamente como definidos pelo contrato que segue.
Saída: somente o JSON definido no contrato que segue.`),
	},
}

// BuiltinPromptSets returns the read-only bundled prompt sets.
func BuiltinPromptSets() []PromptSet {
	sets := make([]PromptSet, len(builtinPromptSets))
	copy(sets, builtinPromptSets)
	return sets
}

// BuiltinPromptSetByID returns a bundled prompt set by identifier.
func BuiltinPromptSetByID(id string) (PromptSet, bool) {
	trimmed := strings.TrimSpace(id)
	for _, set := range builtinPromptSets {
		if set.ID == trimmed {
			return set, true
		}
	}
	return PromptSet{}, false
}

// PersonaMotionBridge tells imported LSO personas how MagicHandy actually drives motion.
const PersonaMotionBridge = `MagicHandy controls the Handy through the "motion" object in your JSON reply.
When the user asks to start, change speed/pattern, or stop the device, you MUST set motion.action in the same JSON response.
Never say motion is handled elsewhere or by another planner.`

// ComposeSystem builds the full system prompt: behavior text from the set,
// then the code-owned contract, then enabled memories when present.
func ComposeSystem(set PromptSet, memories []string) string {
	return ComposeSystemForMode(set, memories, config.MotionGenerationModeProcedural)
}

// ComposeSystemForMode appends motion instructions that match the configured generation mode.
func ComposeSystemForMode(set PromptSet, memories []string, motionGenerationMode string) string {
	var builder strings.Builder
	behavior := strings.TrimSpace(set.System)
	if behavior == "" {
		fallback, _ := BuiltinPromptSetByID(DefaultPromptSetID)
		behavior = fallback.System
	}
	builder.WriteString(behavior)
	if strings.HasPrefix(strings.TrimSpace(set.ID), "persona:") {
		builder.WriteString("\n\n")
		builder.WriteString(PersonaMotionBridge)
	}
	builder.WriteString("\n\n")
	builder.WriteString(motionInstructionsForMode(motionGenerationMode))

	if len(memories) > 0 {
		builder.WriteString("\n\n")
		builder.WriteString(memoryInstructionForPrompt(set.ID))
		for _, memoryText := range memories {
			trimmed := strings.TrimSpace(memoryText)
			if trimmed == "" {
				continue
			}
			builder.WriteString("\n- ")
			builder.WriteString(trimmed)
		}
	}
	return builder.String()
}

func memoryInstructionForPrompt(promptID string) string {
	switch strings.TrimSpace(promptID) {
	case PromptSetIDSpanish:
		return "Memorias guardadas del usuario (haz referencia a ellas con naturalidad cuando sean relevantes; nunca recites la lista):"
	case PromptSetIDPortugueseBrazil:
		return "Memórias salvas do usuário (use-as com naturalidade quando forem relevantes; nunca recite a lista):"
	case PromptSetIDSimplifiedChinese:
		return "已保存的用户记忆（相关时自然引用；不要逐条背诵列表）："
	case PromptSetIDJapanese:
		return "保存済みのユーザーメモリ（関連する場合だけ自然に参照し、一覧を読み上げないこと）:"
	default:
		if strings.HasPrefix(promptID, "persona:") {
			return "Memórias salvas do usuário (use-as com naturalidade quando forem relevantes; nunca recite a lista):"
		}
		return "Saved user memories (reference naturally when relevant; never recite the list):"
	}
}

// RepairPrompt asks the same model to convert malformed output into the contract.
func RepairPrompt(prompt PromptSet, raw string, parseError string) string {
	return fmt.Sprintf(`Repair your previous MagicHandy response.

Return exactly one JSON object matching the contract from the system prompt. Do not add markdown, comments, code fences, or extra keys. Preserve the reply language required by the selected prompt set.

Validation error:
%s

Prompt set:
%s

Previous response:
%s`, strings.TrimSpace(parseError), prompt.ID, strings.TrimSpace(raw))
}
