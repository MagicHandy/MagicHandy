// Package chatapp coordinates conversation use cases above chat persistence.
// It owns session admission and turn lifetimes, never HTTP or device dispatch.
package chatapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

var (
	// ErrReplyActive rejects conversation changes during an interactive turn.
	ErrReplyActive = errors.New("wait for the active reply to finish")
	// ErrAutopilotActive rejects conversation changes during autonomous work.
	ErrAutopilotActive = errors.New("stop Autopilot")
	// ErrTurnInvalidated rejects admission overtaken by application Stop.
	ErrTurnInvalidated = errors.New("chat request was invalidated by Stop")
)

// Sessions is the persistence port used by conversation lifecycle operations.
// Message generation, transport payloads, and HTTP responses are outside it.
type Sessions interface {
	ActiveSessionID() (string, error)
	Sessions() ([]chat.Session, error)
	CreateSession(bool) (chat.Session, error)
	ActivateSession(string, bool) (chat.Session, error)
	SaveSession(string) (chat.Session, error)
	DeleteSession(string) error
	LatestSeqSession(string) (int64, error)
	PromptContext(string) (chat.SessionPromptContext, error)
	AppendTo(string, string, string, string, *chat.MessageDiagnostics) (int64, error)
	ReconcileShutdown(bool) error
}

// Workspace serializes session changes against turn admission. turnsMu never
// spans persistence or mode calls: Stop can cancel a turn while a session
// operation is blocked on disk. Cancellation epochs reject pending admissions;
// turn IDs keep late cleanup from releasing a newer request.
type Workspace struct {
	sessions        Sessions
	autopilotActive func() bool
	lifetime        context.Context
	lifecycleMu     sync.Mutex
	turnsMu         sync.Mutex
	epoch           uint64
	nextID          uint64
	activeID        uint64
	activeCancel    context.CancelFunc
}

// NewWorkspace borrows persistence and the application lifetime. A nil mode
// accessor means autonomous work is unavailable. Callers supply non-nil context
// and persistence; the datastore remains owned by the process composition root.
func NewWorkspace(lifetime context.Context, sessions Sessions, autopilotActive func() bool) *Workspace {
	return &Workspace{sessions: sessions, autopilotActive: autopilotActive, lifetime: lifetime}
}

// BeginTurn admits one interactive request against the canonical active session.
func (w *Workspace) BeginTurn(parent context.Context, sessionID string) (context.Context, context.CancelFunc, error) {
	w.turnsMu.Lock()
	admission := w.epoch
	w.turnsMu.Unlock()
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if err := parent.Err(); err != nil {
		return nil, nil, err
	}
	if err := w.lifetime.Err(); err != nil {
		return nil, nil, err
	}
	if _, err := w.resolveActive(sessionID); err != nil {
		return nil, nil, err
	}
	w.turnsMu.Lock()
	defer w.turnsMu.Unlock()
	if err := parent.Err(); err != nil {
		return nil, nil, err
	}
	if err := w.lifetime.Err(); err != nil {
		return nil, nil, err
	}
	if w.epoch != admission {
		return nil, nil, ErrTurnInvalidated
	}
	if w.activeCancel != nil {
		return nil, nil, errors.New("one chat reply is already active")
	}
	ctx, cancel := context.WithCancel(parent)
	stopLifetime := context.AfterFunc(w.lifetime, cancel)
	w.nextID++
	id := w.nextID
	w.activeID, w.activeCancel = id, cancel
	return ctx, sync.OnceFunc(func() {
		stopLifetime()
		cancel()
		w.turnsMu.Lock()
		if w.activeID == id {
			w.activeCancel = nil
		}
		w.turnsMu.Unlock()
	}), nil
}

// CancelTurns never waits on the session gate, persistence, or mode teardown.
func (w *Workspace) CancelTurns() {
	if w == nil {
		return
	}
	w.turnsMu.Lock()
	w.epoch++
	cancel := w.activeCancel
	w.activeCancel = nil
	w.turnsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// TurnActive reports admission state without waiting for storage.
func (w *Workspace) TurnActive() bool {
	w.turnsMu.Lock()
	defer w.turnsMu.Unlock()
	return w.activeCancel != nil
}

// BeginModeChange holds the session boundary while the edge performs a mode
// transition. The edge may take the persona mutation lock after this lease,
// never in reverse order. Physical Stop never needs this lease.
func (w *Workspace) BeginModeChange() func() {
	w.lifecycleMu.Lock()
	return sync.OnceFunc(w.lifecycleMu.Unlock)
}

// BeginPersonaChange admits a stable conversation/persona mutation. The caller
// performs the persona-domain operation and releases the returned lease.
func (w *Workspace) BeginPersonaChange() (func(), error) {
	w.lifecycleMu.Lock()
	if err := w.requireIdle("changing personas"); err != nil {
		w.lifecycleMu.Unlock()
		return nil, err
	}
	return sync.OnceFunc(w.lifecycleMu.Unlock), nil
}

func (w *Workspace) requireIdle(action string) error {
	if w.TurnActive() {
		return fmt.Errorf("%w before %s", ErrReplyActive, action)
	}
	if w.autopilotActive != nil && w.autopilotActive() {
		return fmt.Errorf("%w before %s", ErrAutopilotActive, action)
	}
	return nil
}

// ResolveActive checks an optional requested ID against the canonical session.
func (w *Workspace) ResolveActive(requested string) (string, error) {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	return w.resolveActive(requested)
}

func (w *Workspace) resolveActive(requested string) (string, error) {
	activeID, err := w.sessions.ActiveSessionID()
	if err != nil {
		return "", errors.New("chat session is unavailable")
	}
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != activeID {
		return "", errors.New("the selected chat is no longer active; refresh the conversation tabs")
	}
	return activeID, nil
}

// Sessions returns a stable list with one canonical active session.
func (w *Workspace) Sessions() ([]chat.Session, error) {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	return w.sessions.Sessions()
}

// Create starts a conversation only when neither chat nor Autopilot owns it.
// The result is the session list observed in the same mutation boundary.
func (w *Workspace) Create(discard bool) ([]chat.Session, error) {
	return w.mutate("starting a new chat", func() error {
		_, err := w.sessions.CreateSession(discard)
		return err
	})
}

// Activate switches the canonical session after checking conversation admission.
func (w *Workspace) Activate(id string, discard bool) ([]chat.Session, error) {
	return w.mutate("switching chats", func() error {
		_, err := w.sessions.ActivateSession(id, discard)
		return err
	})
}

// Save retains a session without interrupting an active reply.
func (w *Workspace) Save(id string) ([]chat.Session, error) {
	return w.mutate("", func() error { _, err := w.sessions.SaveSession(id); return err })
}

// Delete removes an inactive session; persistence enforces active-row protection.
func (w *Workspace) Delete(id string) ([]chat.Session, error) {
	return w.mutate("", func() error { return w.sessions.DeleteSession(id) })
}

func (w *Workspace) mutate(idleAction string, apply func() error) ([]chat.Session, error) {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if idleAction != "" {
		if err := w.requireIdle(idleAction); err != nil {
			return nil, err
		}
	}
	if err := apply(); err != nil {
		return nil, err
	}
	return w.sessions.Sessions()
}

// ReconcileShutdown applies the saved retention policy after work is quiesced.
func (w *Workspace) ReconcileShutdown(keepUnsaved bool) error {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	return w.sessions.ReconcileShutdown(keepUnsaved)
}
