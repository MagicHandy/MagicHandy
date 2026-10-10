package httpapi

import "context"

type hostedChatGuardKey struct{}
type hostedChatGuard struct {
	revision uint64
	mode     string
}

func (s *Server) guardHostedChat(ctx context.Context) context.Context {
	s.cloudPlanning.mu.Lock()
	guard := hostedChatGuard{revision: s.cloudPlanning.revision}
	s.cloudPlanning.mu.Unlock()
	if s.modes != nil {
		guard.mode = s.modes.Status().Mode
	}
	return context.WithValue(ctx, hostedChatGuardKey{}, guard)
}

func (s *Server) hostedChatRetired(ctx context.Context) bool {
	guard, ok := ctx.Value(hostedChatGuardKey{}).(hostedChatGuard)
	if !ok {
		return false
	}
	s.cloudPlanning.mu.Lock()
	retired := guard.revision != s.cloudPlanning.revision
	s.cloudPlanning.mu.Unlock()
	return retired || (s.modes != nil && guard.mode != s.modes.Status().Mode)
}
