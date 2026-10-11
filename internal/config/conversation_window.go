package config

// ConversationWindow is an application budget, not a claim about a provider's
// maximum context. Unconfigured connections keep the previous bounded policy.
// These byte budgets conservatively admit at most one input token per byte.
func (c ModelConnection) ConversationWindow() (contextSize, messages, historyBytes int) {
	switch c.ContextWindow {
	case 131072:
		return 131072, 256, 96000
	case 262144:
		return 262144, 512, 192000
	default:
		return 32768, 64, 24000
	}
}
