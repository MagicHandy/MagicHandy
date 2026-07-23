package httpapi

import (
	"net/http"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/motion/semantic"
)

func (s *Server) shouldUseDirectorChatMode(settings config.Settings) bool {
	return settings.LLM.DirectorMode &&
		settings.Motion.MotionGenerationMode == config.MotionGenerationModeProcedural
}

func (s *Server) handleChatStreamDirector(
	_ http.ResponseWriter,
	r *http.Request,
	body chatStreamRequest,
	settings config.Settings,
	provider llm.Provider,
	emit sseEmitter,
) {
	ctx := r.Context()
	intent, directorLatency, err := chat.AskDirector(ctx, provider, settings.LLM.Model, body.Message, body.History)
	if err != nil {
		s.logger.Warn("director call failed", "error", err, "latency_ms", directorLatency.Milliseconds())
		_ = emit("error", map[string]string{"message": err.Error()})
		_ = emit("done", map[string]any{"ok": false})
		return
	}

	motion.MotionDebugLog("DIR", "chat_director.go:handleChatStreamDirector", "director_latency_ms", map[string]any{
		"latency_ms": directorLatency.Milliseconds(),
		"action":     intent.Action,
		"location":   intent.Location,
		"intensity":  intent.Intensity,
	})

	if err := emit("intent", map[string]any{
		"action":    intent.Action,
		"location":  intent.Location,
		"intensity": intent.Intensity,
	}); err != nil {
		return
	}

	prefs := semantic.LoadMotionPreferences(settings.Motion.MotionPreferences)
	motionRunning := s.chatChaosActive()
	command, err := chat.MotionCommandFromLLMIntent(intent, prefs, motionRunning)
	if err != nil {
		_ = emit("error", map[string]string{"message": err.Error()})
		_ = emit("done", map[string]any{"ok": false})
		return
	}

	dispatch := s.dispatchChatChaoticMotionAsync(ctx, command, settings)
	if dispatch.Applied || dispatch.Error != "" {
		if err := emit("motion", dispatch); err != nil {
			return
		}
	}

	var replyBuilder strings.Builder
	reply, err := chat.AskActor(ctx, provider, settings.LLM.Model, body.Message, body.History, intent, func(token string) error {
		replyBuilder.WriteString(token)
		return emit("delta", map[string]any{
			"phase": "actor",
			"text":  token,
		})
	})
	if err != nil {
		s.logger.Warn("actor stream failed", "error", err)
		_ = emit("error", map[string]string{"message": err.Error()})
		_ = emit("done", map[string]any{"ok": false})
		return
	}
	if reply == "" {
		reply = replyBuilder.String()
	}

	if err := emit("message", map[string]any{
		"reply":  reply,
		"motion": command,
	}); err != nil {
		return
	}

	_ = emit("done", map[string]any{"ok": true})
}
