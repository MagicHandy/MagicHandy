package httpapi

import (
	"context"
	"errors"
	"time"
)

func (s *Server) chatRepairCancellationOutcome(ctx context.Context, stop uint64, failure error) string {
	if s.stopSequence.Load() != stop {
		return "stopped"
	}
	if errors.Is(context.Cause(ctx), errLLMSettingsChanged) || errors.Is(failure, errLLMSettingsChanged) || errors.Is(failure, errChatRepairStale) {
		return "stale"
	}
	if ctx.Err() != nil {
		return "canceled"
	}
	if errors.Is(failure, errChatRepairStale) || s.hostedChatRetired(ctx) || s.chatRepairRetired(ctx) {
		return "stale"
	}
	return ""
}

// The late publication checks share the same distinction as the entry check:
// changed motion/settings are an expired repair, not an Emergency Stop.
func (s *Server) emitChatCancellationAt(ctx context.Context, stop uint64, emit sseEmitter, replyRetained bool) {
	if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok && (candidate.active.Load() || candidate.evidenceID.Load() > 0) {
		outcome := s.chatRepairCancellationOutcome(ctx, stop, nil)
		s.recordLateRepairCancellation(ctx, candidate, outcome)
		if outcome == "stale" || outcome == "canceled" {
			message := errChatRepairStale.Error()
			if outcome == "canceled" {
				message = s.chatInterruptionMessage(ctx, stop, context.Canceled)
			}
			payload := map[string]string{"message": message}
			if replyRetained {
				payload["message"] = chatPostReplyInterruptedMessage
				payload["reply_retained"] = "true"
			}
			_ = emit("error", payload)
			_ = emit("done", map[string]any{"ok": false})
			return
		}
	}
	if s.stopSequence.Load() != stop {
		if s.lastStopWasPositionReset() {
			payload := map[string]string{"message": chatPositionResetMessage}
			if replyRetained {
				payload["message"], payload["reply_retained"] = chatPositionResetAfterReply, "true"
			}
			_ = emit("error", payload)
			_ = emit("done", map[string]any{"ok": false})
			return
		}
		if replyRetained {
			emitChatPostReplyCancellation(emit)
		} else {
			emitChatCancellation(emit)
		}
		return
	}
	message := s.chatInterruptionMessage(ctx, stop, context.Canceled)
	payload := map[string]string{"message": message}
	if replyRetained {
		payload["message"], payload["reply_retained"] = chatPostReplyInterruptedMessage, "true"
	}
	_ = emit("error", payload)
	_ = emit("done", map[string]any{"ok": false})
}

func (s *Server) recordLateRepairCancellation(ctx context.Context, candidate *chatRepairCandidate, outcome string) {
	id := candidate.evidenceID.Load()
	if id == 0 || outcome == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	if err := s.chatLog.SetFailureRepairOutcome(ctx, candidate.admission.session, id, outcome); err != nil {
		s.logger.Warn("chat repair cancellation diagnostics unavailable")
	}
}
