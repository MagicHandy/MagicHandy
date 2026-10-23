package httpapi

import (
	"context"
	"errors"
	"fmt"
)

// Causes describe application events, never inferred words in a conversation.
// They keep errors.Is(context.Canceled) working for lifecycle callers.
var (
	errLLMSettingsChanged = fmt.Errorf("model settings changed: %w", context.Canceled)
	errChatControlChanged = fmt.Errorf("controller changed or disconnected: %w", context.Canceled)
	errChatAccessEnded    = fmt.Errorf("sign-in session ended: %w", context.Canceled)
	errChatServerStopped  = fmt.Errorf("application shutting down: %w", context.Canceled)
)

const (
	chatInterruptedMessage          = "The reply was interrupted before it finished. Please try again."
	chatTimedOutMessage             = "The model took too long to reply. Please try again."
	chatChangedMessage              = "The model or motion settings changed while the reply was being prepared. Please send your request again."
	chatControlChangedMessage       = "The reply was interrupted because this browser lost control of the session. Take control, then try again."
	chatAccessEndedMessage          = "Your sign-in session ended before the reply finished. Sign in again to continue."
	chatServerStoppedMessage        = "MagicHandy is shutting down, so this reply could not finish."
	chatPostReplyInterruptedMessage = "The reply was saved, but its motion or speech was interrupted. Check the current controls before trying again."
)

func (s *Server) chatInterruptionMessage(ctx context.Context, stop uint64, failure error) string {
	if s.stopSequence.Load() != stop {
		return "Chat canceled by Emergency Stop."
	}
	cause := context.Cause(ctx)
	for _, item := range []struct {
		cause   error
		message string
	}{
		{errLLMSettingsChanged, chatChangedMessage},
		{errChatControlChanged, chatControlChangedMessage},
		{errChatAccessEnded, chatAccessEndedMessage},
		{errChatServerStopped, chatServerStoppedMessage},
		{context.DeadlineExceeded, chatTimedOutMessage},
	} {
		if errors.Is(cause, item.cause) || errors.Is(failure, item.cause) {
			return item.message
		}
	}
	if errors.Is(failure, errChatRepairStale) {
		return errChatRepairStale.Error()
	}
	// An ended request is not itself evidence that repair inputs changed.
	// Do not detach it merely to revalidate repair state for an error message.
	if ctx.Err() != nil {
		return chatInterruptedMessage
	}
	if s.chatRepairRetired(ctx) {
		return errChatRepairStale.Error()
	}
	if s.hostedChatRetired(ctx) {
		return chatChangedMessage
	}
	if errors.Is(cause, context.Canceled) || errors.Is(failure, context.Canceled) {
		return chatInterruptedMessage
	}
	return ""
}

// The public error text is redacted, while its typed cause remains available
// for lifecycle classification. Error never formats the private cause.
type retainedChatFailure struct {
	message string
	cause   error
}

func (e retainedChatFailure) Error() string { return e.message }
func (e retainedChatFailure) Unwrap() error { return e.cause }

func sharedChatInterruption(message string) bool {
	switch message {
	case chatInterruptedMessage, chatTimedOutMessage, chatChangedMessage, chatControlChangedMessage,
		chatAccessEndedMessage, chatServerStoppedMessage, chatPostReplyInterruptedMessage,
		"Chat canceled by Emergency Stop.",
		"Emergency Stop canceled this reply's motion and speech.":
		return true
	}
	return false
}
