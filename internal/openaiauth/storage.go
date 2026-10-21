package openaiauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// A non-JSON envelope makes older versions fail closed rather than treating
// encrypted credentials as an empty JSON object and overwriting registrations.
const sealedCredentialHeader = "MAGICHANDY-SEALED-CREDENTIALS\nv1:windows-dpapi\n"
const maxCredentialBytes = 1 << 20

var errUnreadableCredentials = errors.New("saved cloud credentials cannot be opened; reset saved credentials in model settings, then reconnect your providers")

// transaction is called with mu held and never performs network I/O. A separate
// refresh lock serializes rotating tokens across processes without blocking reads.
func (m *Manager) transaction(ctx context.Context, action func(*credentialFile) error) error {
	unlock, err := m.storageLock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	data, plain, migrate, err := readCredentials(m.path)
	defer clear(plain)
	if err != nil {
		return err
	}
	if err := initializeCredentialHost(&data); err != nil {
		return err
	}
	if err := action(&data); err != nil {
		return err
	}
	encoded, err := json.Marshal(data) // #nosec G117 -- host-only credential serialization; never a public response or log.
	if err != nil || len(encoded) > maxCredentialBytes {
		return errors.New("OpenAI credentials could not be saved")
	}
	defer clear(encoded)
	if !migrate && bytes.Equal(plain, encoded) {
		return nil
	}
	return m.writeCredentials(encoded)
}

func (m *Manager) storageLock(ctx context.Context) (func(), error) {
	if m.closed || ctx.Err() != nil {
		return nil, context.Canceled
	}
	if !m.storageReady {
		directory := filepath.Dir(m.path)
		if err := os.MkdirAll(directory, 0700); err != nil {
			return nil, errors.New("create protected OpenAI storage")
		}
		if err := restrictPath(directory); err != nil {
			return nil, errors.New("protect OpenAI storage")
		}
		m.storageReady = true
	}
	unlock, err := lockCredentials(ctx, m.path+".lock")
	if err != nil {
		return nil, errors.New("OpenAI credentials are busy or unavailable")
	}
	return unlock, nil
}

func readCredentials(path string) (credentialFile, []byte, bool, error) {
	data := credentialFile{Profiles: []registration{}}
	file, err := os.Open(path) // #nosec G304 -- fixed host-owned credential path, never a request/import path.
	if os.IsNotExist(err) {
		return data, nil, false, nil
	}
	if err != nil {
		return data, nil, false, errors.New("OpenAI credentials could not be read")
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxCredentialBytes+65537))
	defer clear(raw)
	if err != nil {
		return data, nil, false, errors.New("OpenAI credentials could not be read")
	}
	if len(raw) > maxCredentialBytes+65536 {
		return data, nil, false, errUnreadableCredentials
	}
	plain, migrate, err := decodeCredentials(raw)
	if err != nil {
		return data, nil, false, errUnreadableCredentials
	}
	trimmed := bytes.TrimSpace(plain)
	if len(plain) > maxCredentialBytes || len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(plain, &data) != nil {
		clear(plain)
		return credentialFile{}, nil, false, errUnreadableCredentials
	}
	return data, plain, migrate, nil
}

func (m *Manager) writeCredentials(plain []byte) error {
	encoded, err := encodeCredentials(plain)
	if err != nil {
		return errors.New("OpenAI credentials could not be protected")
	}
	defer clear(encoded)
	temporary, err := os.CreateTemp(filepath.Dir(m.path), "credentials-*.tmp")
	if err != nil {
		return errors.New("OpenAI credentials could not be saved")
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	if err := restrictPath(temporary.Name()); err != nil {
		_ = temporary.Close()
		return errors.New("OpenAI credentials could not be protected")
	}
	_, writeErr := temporary.Write(encoded)
	if writeErr == nil {
		writeErr = temporary.Sync()
	}
	closeErr := temporary.Close()
	if writeErr != nil || closeErr != nil || os.Rename(temporary.Name(), m.path) != nil {
		return errors.New("OpenAI credentials could not be saved")
	}
	return nil
}

// ResetUnreadableCredentials is an explicit recovery operation. It never resets
// a readable store (including one another app instance has already repaired).
// Provider definitions and model routing are stored elsewhere and stay intact.
func (m *Manager) ResetUnreadableCredentials(ctx context.Context) error {
	m.mu.Lock()
	err := m.resetUnreadableLocked(ctx)
	if err == nil {
		m.invalidateLocked()
		if m.pending != nil {
			m.pending.cancel()
			m.pending = nil
		}
		m.state, m.message = "disconnected", ""
	}
	m.mu.Unlock()
	if err == nil {
		m.notifyChange()
	}
	return err
}

func (m *Manager) resetUnreadableLocked(ctx context.Context) error {
	unlock, err := m.storageLock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	_, plain, _, readErr := readCredentials(m.path)
	clear(plain)
	if !errors.Is(readErr, errUnreadableCredentials) {
		return errors.New("saved cloud credentials do not need resetting; refresh settings")
	}
	data := credentialFile{Profiles: []registration{}}
	if err := initializeCredentialHost(&data); err != nil {
		return err
	}
	encoded, err := json.Marshal(data) // #nosec G117 -- empty credential store containing only a new host identity.
	if err != nil {
		return errors.New("OpenAI credentials could not be reset")
	}
	defer clear(encoded)
	return m.writeCredentials(encoded)
}
