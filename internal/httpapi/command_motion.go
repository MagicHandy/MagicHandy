package httpapi

import (
	"context"
	"errors"
	"strings"
)

func deferredMotionCommand(route string) bool {
	route, _, _ = strings.Cut(route, "?")
	return route == "/api/chat/stream" || route == "/api/labs/llm/chat"
}

// Only commands that can change motion supersede a pending model proposal.
// Voice playback acknowledgments, chat cursors and file operations do not.
func motionCommandRoute(route string) bool {
	route, _, _ = strings.Cut(route, "?")
	switch route {
	case "/api/chat/stream", "/api/labs/llm/chat", "/api/labs/llm/session",
		"/api/motion/start", "/api/motion/target", "/api/motion/quick", "/api/motion/pause", "/api/motion/resume",
		"/api/settings", "/api/settings/reset", "/api/settings/labs", "/api/settings/llm-motion-mode", "/api/settings/device/connection-key",
		"/api/media/sync", "/api/media/script-offset", "/api/media/playback":
		return true
	}
	return strings.HasPrefix(route, "/api/modes/") ||
		(strings.HasPrefix(route, "/api/library/") && strings.HasSuffix(route, "/play")) ||
		(strings.HasPrefix(route, "/api/transport/") && (strings.HasSuffix(route, "/connect") || strings.HasSuffix(route, "/disconnect") || strings.HasSuffix(route, "/select")))
}

// Admission and application share this lane with immediate control commands.
// A model response keeps its text, but cannot apply an older motion intention
// after a newer accepted action. Stop bypasses the lane and cancels the context.
func (s *Server) beginDeferredMotion(ctx context.Context) (func(), error) {
	invocation, ok := ctx.Value(commandInvocationKey{}).(*commandInvocation)
	if !ok || invocation.receipt == nil || !deferredMotionCommand(invocation.receipt.route) {
		return func() {}, nil
	}
	release, err := s.commands.acquire(ctx)
	if err != nil {
		return nil, err
	}
	s.commands.mu.Lock()
	current := s.commands.scope == invocation.scope && s.commands.motionSequence == invocation.receipt.Sequence
	s.commands.mu.Unlock()
	if !current || ctx.Err() != nil {
		release()
		return nil, errors.New("a newer control command superseded this reply's motion")
	}
	return release, nil
}
