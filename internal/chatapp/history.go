package chatapp

import (
	"context"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

// Observation is one coherent read of the active conversation's status.
type Observation struct {
	ActiveSessionID string
	LatestSeq       int64
	CurrentMood     chat.Mood
}

// Observe reads status without letting a session switch split the observation.
func (w *Workspace) Observe(ctx context.Context, trackMood bool) (Observation, error) {
	ctx, cancel := w.withLifetime(ctx)
	defer cancel()
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	var state Observation
	var err error
	state.ActiveSessionID, err = w.sessions.ActiveSessionIDContext(ctx)
	if err != nil {
		return state, err
	}
	state.LatestSeq, err = w.sessions.LatestSeqSessionContext(ctx, state.ActiveSessionID)
	if err != nil || !trackMood {
		return state, err
	}
	prompt, err := w.sessions.ReadPromptContext(ctx, state.ActiveSessionID)
	if err == nil {
		state.CurrentMood = prompt.CurrentMood
	}
	return state, err
}

// StopRecord contains the transcript metadata accepted after a completed Stop.
type StopRecord struct {
	SessionID   string
	UserSeq     int64
	ReplySeq    int64
	Diagnostics chat.MessageDiagnostics
}

// RecordStoppedReply is best-effort history after physical Stop has completed
// or timed out. It cannot dispatch motion or speech, and stale session IDs never
// redirect the transcript into the new active chat.
func (w *Workspace) RecordStoppedReply(requestedID, message, reply, clientID string, trackMood bool, diagnostics chat.MessageDiagnostics) (StopRecord, error) {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	record := StopRecord{Diagnostics: diagnostics}
	activeID, err := w.resolveActive(context.Background(), requestedID)
	if err != nil {
		return record, err
	}
	record.SessionID = activeID
	record.UserSeq, err = w.sessions.AppendTo(activeID, chat.MessageRoleUser, message, clientID, nil)
	if trackMood {
		prompt, moodErr := w.sessions.ReadPromptContext(context.Background(), activeID)
		if moodErr == nil {
			record.Diagnostics.Mood = prompt.CurrentMood
		}
		err = errors.Join(err, moodErr)
	}
	var replyErr error
	record.ReplySeq, replyErr = w.sessions.AppendTo(activeID, chat.MessageRoleAssistant, reply, "", &record.Diagnostics)
	return record, errors.Join(err, replyErr)
}
