package httpapi

import (
	"context"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// autopilotStandingHold remembers that a live continuous-chat turn read the
// human as wanting the current motion kept exactly as it is. The model makes
// that judgment from the whole conversation when the human speaks; this only
// remembers the answer. Planning turns alone could not carry a standing wish:
// in live sessions they honored it for a turn or two and then drifted after an
// unrelated remark. The hold is keyed to the chat session, so a new
// conversation never inherits it, and it lasts until a later chat turn reads a
// wish for change or a fresh Autopilot run starts.
type autopilotStandingHold struct {
	mu        sync.Mutex
	sessionID string
}

func (h *autopilotStandingHold) record(sessionID string, stay bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if stay {
		h.sessionID = sessionID
	} else if h.sessionID == sessionID {
		h.sessionID = ""
	}
}

func (h *autopilotStandingHold) holds(sessionID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return sessionID != "" && h.sessionID == sessionID
}

func (h *autopilotStandingHold) release() {
	h.mu.Lock()
	h.sessionID = ""
	h.mu.Unlock()
}

// chatTurnMotion is the motion state for a live chat turn. It also says whether
// continuous Autopilot is composing the motion and whether it holds it at the
// human's request; only then is a standing wish offered to the model.
func (s *Server) chatTurnMotion(settings config.Settings, requests []chat.RecentUserRequest, sessionID string) chat.MotionContext {
	state := s.contextualChatMotion(settings, requests)
	if s.modes == nil || !continuousChatMode(settings.LLM.MotionGenerationMode) {
		return state
	}
	if status := s.modes.Status(); !status.Active || status.Mode != modes.ModeAutopilot {
		return state
	}
	state.Autopilot = true
	state.StandingHold = s.autopilotHold.holds(sessionID)
	state.EarlierScores = chatEarlierScores(s.modes.EarlierScores())
	return state
}

// chatEarlierScores passes the run's earlier distinct scores to a model turn.
func chatEarlierScores(scores []modes.EarlierScore) []chat.EarlierScore {
	out := make([]chat.EarlierScore, 0, len(scores))
	for _, score := range scores {
		if score.Flow != nil {
			out = append(out, chat.EarlierScore{Flow: *motion.CloneFlowSpec(score.Flow), StartedSecondsAgo: score.StartedSecondsAgo, PlayedSeconds: score.PlayedSeconds})
		}
	}
	return out
}

// afterLiveChatTurn applies what a committed live chat turn means for
// continuous Autopilot. It remembers a declared standing wish. When the turn
// left the motion unchanged, the planner reconsiders now: live chat decides its
// action before writing its reply, so it often answers a remark such as "that's
// too much" in words alone, while the planner acts on it.
func (s *Server) afterLiveChatTurn(sessionID string, response chat.AssistantResponse) {
	if stay := response.StayUnchanged; stay != nil {
		s.autopilotHold.record(sessionID, *stay)
	}
	settings, _ := s.store.Snapshot()
	if response.Motion == nil && s.modes != nil && continuousChatMode(settings.LLM.MotionGenerationMode) {
		s.modes.ReconsiderAfterChat()
	}
}

// requestedAutopilotHold answers a planning boundary without inference while
// the human's standing wish applies. Only the continuous modes declare one.
func (s *Server) requestedAutopilotHold(ctx context.Context) (modes.Decision, bool) {
	settings, _ := s.store.Snapshot()
	if !continuousChatMode(settings.LLM.MotionGenerationMode) {
		return modes.Decision{}, false
	}
	sessionID, err := s.chatLog.ActiveSessionIDContext(ctx)
	if err != nil || !s.autopilotHold.holds(sessionID) {
		return modes.Decision{}, false
	}
	return modes.Decision{Hold: true, Requested: true, Next: modes.TimingNormal}, true
}
