//go:build !windows

package openaiauth

import (
	"bytes"
	"slices"
)

// Non-Windows hosts retain owner-only file protection. Do not pretend a fixed
// application key is encryption, or silently replace a Windows-encrypted store.
func encodeCredentials(plain []byte) ([]byte, error) { return slices.Clone(plain), nil }

func decodeCredentials(raw []byte) ([]byte, bool, error) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '{' {
		return slices.Clone(raw), false, nil
	}
	return nil, false, errUnreadableCredentials
}
