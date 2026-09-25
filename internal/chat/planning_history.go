package chat

import (
	"fmt"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// PlanningHistory marks each human line in a continuous planning turn's
// history with how long ago it was said. Planning replies are never dialogue,
// so the person's last line stays the latest user turn for minutes. Without the
// age the model read an old "that's too much" as a new reaction to the pace it
// had chosen since, and cut the pace again. Only human lines are marked, and a
// planning reply is never published, so the mark is not spoken. When any time
// is unknown, the history is returned unchanged.
func PlanningHistory(history []llm.Message, at []time.Time, now time.Time) []llm.Message {
	if len(at) != len(history) || now.IsZero() {
		return history
	}
	marked := make([]llm.Message, len(history))
	for i, message := range history {
		marked[i] = message
		if message.Role != MessageRoleUser {
			continue
		}
		if at[i].IsZero() {
			return history
		}
		marked[i].Content = fmt.Sprintf("[said %s ago] %s", formatSessionSpan(max(0, int(now.Sub(at[i])/time.Second))), message.Content)
	}
	return marked
}
