package config

// MinimumExternalContext leaves room for compound output contracts without
// increasing the compact tier's retained conversation history.
const MinimumExternalContext = 65536

// ConversationWindow is an application budget, not a claim about a provider's
// maximum context. History may be shorter than the total request budget, leaving
// room for a rich motion state without sending a longer conversation. Omitted
// history settings preserve existing connections' behavior. Neither setting
// changes local-model defaults. Byte budgets assume one input token per byte.
func (c ModelConnection) ConversationWindow() (contextSize, messages, historyBytes int) {
	switch c.ContextWindow {
	case 131072:
		contextSize, messages, historyBytes = 131072, 256, 96000
	case 262144:
		contextSize, messages, historyBytes = 262144, 512, 192000
	default:
		contextSize, messages, historyBytes = MinimumExternalContext, 64, 24000
	}
	switch c.HistorySize {
	case "recent":
		messages, historyBytes = min(messages, 64), min(historyBytes, 24000)
	case "long":
		messages, historyBytes = min(messages, 256), min(historyBytes, 96000)
	}
	return
}
