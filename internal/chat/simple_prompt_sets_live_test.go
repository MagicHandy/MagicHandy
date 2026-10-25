//go:build liveeval

package chat

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Native-language turns exercise the five shipped sets without translating
// user input or changing their stored prose. The same parsed intent checks
// apply to each language; language/tone is inspected from retained replies.
func TestSimplePromptSetsLive(t *testing.T) {
	provider, model, hosted := naturalLiveProvider(t)
	requests := map[string][]string{
		DefaultPromptSetID:           {"Keep strokes between 65 and 100, with naturally varying lengths. Keep the pace.", "Release the upper-only restriction. Mix full-device sweeps with short tip strokes. Keep pace unchanged.", "Now make every stroke cover the entire slider from base to tip. Keep pace unchanged.", "Repeat this exact motion. Change nothing.", "Increase only speed by seven percentage points within my limits."},
		PromptSetIDSpanish:           {"Mantén los recorridos entre 65 y 100, variando su longitud con naturalidad. Conserva la velocidad.", "Elimina la restricción a la zona superior. Combina recorridos completos del dispositivo con recorridos cortos en la punta. Conserva la velocidad.", "Ahora cada recorrido debe cubrir todo el dispositivo, desde la base hasta la punta. Conserva la velocidad.", "Repite exactamente este movimiento. No cambies nada.", "Aumenta solo la velocidad en siete puntos porcentuales, dentro de mis límites."},
		PromptSetIDPortugueseBrazil:  {"Mantenha os movimentos entre 65 e 100, variando o comprimento naturalmente. Mantenha a velocidade.", "Remova a restrição à região superior. Misture movimentos por toda a extensão do dispositivo com movimentos curtos na ponta. Mantenha a velocidade.", "Agora cada movimento deve cobrir toda a extensão, da base até a ponta. Mantenha a velocidade.", "Repita exatamente este movimento. Não mude nada.", "Aumente apenas a velocidade em sete pontos percentuais, dentro dos meus limites."},
		PromptSetIDSimplifiedChinese: {"把运动限制在65到100之间，让行程长度自然变化。保持速度不变。", "取消只在上部运动的限制。把覆盖整个设备的长行程与顶端的短行程混合起来。保持速度不变。", "现在每次行程都要从底部到顶端覆盖整个滑动范围。保持速度不变。", "完全重复当前的运动，不要做任何改动。", "只把速度提高七个百分点，不超过我设置的限制。"},
		PromptSetIDJapanese:          {"動く範囲を65から100に限定し、その中でストロークの長さを自然に変えてください。速度は変えないでください。", "上部だけという制限を解除してください。装置の全範囲を使うストロークと先端での短いストロークを組み合わせ、速度は変えないでください。", "今度は毎回、根元から先端までスライダーの全範囲を使ってください。速度は変えないでください。", "今の動きを完全に同じまま繰り返してください。何も変えないでください。", "設定した制限内で、速度だけを7パーセントポイント上げてください。"},
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 100
	for _, prompt := range BuiltinPromptSets() {
		if selected := os.Getenv("MAGICHANDY_PROMPT_SET_ID"); selected != "" && selected != prompt.ID {
			continue
		}
		for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
			score := DefaultLayeredScore(43)
			if mode == MotionModeCreativeV2 {
				score = FreshCreativeV2Score(43)
			}
			score.Seed = 20261025
			var history []llm.Message
			for i, message := range requests[prompt.ID] {
				t.Run(fmt.Sprintf("%s/%s/%d", prompt.ID, mode, i+1), func(t *testing.T) {
					before := *motion.CloneFlowSpec(&score)
					state := naturalState(&score, mode, limits)
					caps := Capabilities{Motion: true, MotionMode: mode, SimpleMotion: naturalSimple(), IntermediateMotion: naturalIntermediate(), AdvancedMotion: hosted, PreserveConversationText: hosted}
					if os.Getenv("MAGICHANDY_PROMPT_SET_VARIANT") == "japanese_reminder" && prompt.ID == PromptSetIDJapanese {
						caps.SimpleMotion = false
						provider.guide = "変更する項目だけをコンパクトなJSONで出力してください。現在のスコア全体をコピーしないでください。速度だけを変える依頼なら速度だけを編集します。geometryが形状を指定する場合、そのレイヤーを作り直さないでください。依頼された変更をすべて反映し、依頼されていない設定は省略してください。\n範囲の制限を解除して装置全体を使う場合、両端を0と100に戻す必要があります。full_and_tip/baseだけでは以前の範囲が残ります。短いストロークを広げる場合、長いストロークと両端を保ちながら短い方の幅を広げてください。同じgeometryの再指定では幅は広がりません。完全な繰り返しは空のeditsで、evolveもしません。細部を新しくする依頼だけがevolveを許可します。説明の前に実際の編集内容を確認してください。"
					}
					service := Service{Provider: provider, Model: model, Prompt: prompt, Capabilities: &caps, MotionContext: &state, MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget(), ReasoningMode: "off"}
					started, calls := time.Now(), provider.calls
					result, err := service.Complete(t.Context(), Request{Message: message, History: history}, nil)
					if err == nil && result.Response.Motion != nil && result.Response.Motion.Layered != nil {
						score = *motion.CloneFlowSpec(result.Response.Motion.Layered)
					}
					intent := []int{0, 1, 6, 7, 10}[i]
					pass := err == nil && !result.Repaired && !result.SemanticFallback && result.Response.Reply != "" && provider.calls-calls == 1 && naturalConversationIntent(intent, before, score)
					row := hostedLayeredLiveRow(fmt.Sprintf("%s/%s/%d", prompt.ID, mode, i+1), model, message, result.Raw, before, result.Response.Motion, limits, started, err, pass)
					row["method"], row["reply"], row["raw_attempts"], row["provider_calls"] = mode, result.Response.Reply, append([]string(nil), provider.raws[calls:]...), provider.calls-calls
					rows = append(rows, row)
					saveHostedLayeredRows(t, rows)
					history = append(history, llm.Message{Role: "user", Content: message}, llm.Message{Role: "assistant", Content: result.Response.Reply})
					if !pass {
						t.Error("prompt-set intent failed; retained for review")
					}
				})
			}
		}
	}
}
