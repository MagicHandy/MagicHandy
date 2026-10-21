package openaiauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type refreshKey struct {
	id         string
	generation uint64
}

type tokenRefresh struct {
	done   chan struct{}
	cancel context.CancelFunc
	token  string
	err    error
}

func (m *Manager) accessToken(ctx context.Context, id string, generation uint64) (string, error) {
	m.mu.Lock()
	profile, _, err := m.refreshSnapshot(ctx, id, generation)
	if err != nil {
		m.mu.Unlock()
		return "", err
	}
	if !needsRefresh(profile) {
		m.mu.Unlock()
		return profile.AccessToken, err
	}
	key := refreshKey{id, generation}
	if call := m.refreshes[key]; call != nil {
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-call.done:
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.closed || generation != m.generation {
				return "", context.Canceled
			}
			return call.token, call.err
		}
	}
	operation, cancel := m.operationContext(ctx)
	call := &tokenRefresh{done: make(chan struct{}), cancel: cancel}
	m.refreshes[key] = call
	m.wg.Add(1)
	m.mu.Unlock()
	defer m.wg.Done()
	defer cancel()

	token, changed, err := m.refreshToken(operation, id, generation)
	m.mu.Lock()
	if m.closed || generation != m.generation && !changed {
		token, err = "", context.Canceled
	}
	call.token, call.err = token, err
	delete(m.refreshes, key)
	close(call.done)
	m.mu.Unlock()
	if changed {
		m.notifyChange()
	}
	return token, err
}

func needsRefresh(profile registration) bool {
	return time.Now().Add(90 * time.Second).After(profile.ExpiresAt)
}

// Called with mu held, but only while reading the short credential transaction.
func (m *Manager) refreshSnapshot(ctx context.Context, id string, generation uint64) (registration, uint64, error) {
	if m.closed || generation != m.generation || ctx.Err() != nil {
		return registration{}, 0, context.Canceled
	}
	var profile registration
	var revision uint64
	err := m.transaction(ctx, func(data *credentialFile) error {
		if data.Active != id {
			return context.Canceled
		}
		index := slices.IndexFunc(data.Profiles, func(p registration) bool { return p.ID == id })
		if index < 0 || data.Profiles[index].AccessToken == "" {
			return &llm.CloudError{Kind: "signed_out"}
		}
		candidate := data.Profiles[index]
		if !slices.Contains(candidate.Scopes, "chatgpt.tokens.use.direct") {
			return &llm.CloudError{Kind: "permission"}
		}
		if needsRefresh(candidate) && time.Now().Before(candidate.EarliestRefreshAt) {
			return &llm.CloudError{Kind: "unavailable"}
		}
		profile, revision = candidate, data.Revision
		return nil
	})
	return profile, revision, err
}

func (m *Manager) refreshToken(ctx context.Context, id string, generation uint64) (string, bool, error) {
	// Separate per-registration locks preserve cross-process refresh-token
	// rotation. Status, sign-out and account switching never acquire this lock.
	digest := sha256.Sum256([]byte(id))
	unlock, err := lockCredentials(ctx, fmt.Sprintf("%s.refresh-%x.lock", m.path, digest[:16]))
	if err != nil {
		return "", false, errors.New("OpenAI credentials are busy or unavailable")
	}
	defer unlock()
	m.mu.Lock()
	before, revision, err := m.refreshSnapshot(ctx, id, generation)
	m.mu.Unlock()
	if err != nil {
		return "", false, err
	}
	if !needsRefresh(before) {
		return before.AccessToken, false, nil
	}
	after := before
	refreshErr := m.refresh(ctx, &after)
	return m.commitRefresh(ctx, generation, revision, before, after, refreshErr)
}

func (m *Manager) commitRefresh(ctx context.Context, generation, revision uint64, before, after registration, refreshErr error) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || generation != m.generation || ctx.Err() != nil {
		return "", false, context.Canceled
	}
	var failure *llm.CloudError
	signedOut := errors.As(refreshErr, &failure) && failure.Kind == "signed_out"
	if refreshErr != nil && !signedOut {
		return "", false, refreshErr
	}
	if signedOut {
		after.AccessToken, after.RefreshToken, after.IDToken = "", "", ""
	}
	permissionLost := !slices.Contains(after.Scopes, "chatgpt.tokens.use.direct")
	err := m.transaction(ctx, func(data *credentialFile) error {
		index := slices.IndexFunc(data.Profiles, func(p registration) bool { return p.ID == before.ID })
		if data.Active != before.ID || data.Revision != revision || index < 0 {
			return context.Canceled
		}
		// Acknowledging the welcome notice does not change authorization. Keep
		// it without discarding a successfully rotated, single-use token.
		current := data.Profiles[index]
		after.WelcomeAcknowledged = current.WelcomeAcknowledged
		current.WelcomeAcknowledged = before.WelcomeAcknowledged
		if !reflect.DeepEqual(current, before) {
			return context.Canceled
		}
		data.Profiles[index] = after
		if signedOut || permissionLost {
			data.Revision++
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	if signedOut || permissionLost {
		m.invalidateLocked()
		if signedOut {
			return "", true, refreshErr
		}
		return "", true, &llm.CloudError{Kind: "permission"}
	}
	return after.AccessToken, false, nil
}
