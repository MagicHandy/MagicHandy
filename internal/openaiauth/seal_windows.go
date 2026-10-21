//go:build windows

package openaiauth

import (
	"bytes"
	"slices"
	"unsafe"

	"golang.org/x/sys/windows"
)

func encodeCredentials(plain []byte) ([]byte, error) {
	sealed, err := protectCredentialBytes(plain, false)
	if err != nil {
		return nil, err
	}
	return append([]byte(sealedCredentialHeader), sealed...), nil
}

func decodeCredentials(raw []byte) ([]byte, bool, error) {
	if bytes.HasPrefix(raw, []byte(sealedCredentialHeader)) {
		plain, err := protectCredentialBytes(raw[len(sealedCredentialHeader):], true)
		return plain, false, err
	}
	// Only a legacy JSON object may migrate. Unknown encrypted formats fail
	// closed, including future envelope versions.
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '{' {
		return slices.Clone(raw), true, nil
	}
	return nil, false, errUnreadableCredentials
}

// DPAPI defaults to the current Windows user. Never set LOCAL_MACHINE: that
// would let other users on this PC decrypt the store. UI_FORBIDDEN prevents a
// background status request from opening an operating-system prompt.
func protectCredentialBytes(input []byte, decrypt bool) ([]byte, error) {
	if len(input) == 0 || len(input) > maxCredentialBytes+65536 {
		return nil, errUnreadableCredentials
	}
	in := windows.DataBlob{Size: uint32(len(input)), Data: &input[0]} // #nosec G115 -- input is bounded to just over 1 MiB above.
	var out windows.DataBlob
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	// The Windows API owns this allocation. Copy it before wiping and freeing;
	// no pointer into OS memory escapes this function.
	buffer := unsafe.Slice(out.Data, out.Size) // #nosec G103 -- DPAPI-owned buffer and length, copied before LocalFree below.
	result := slices.Clone(buffer)
	clear(buffer)
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) // #nosec G103 -- DPAPI requires LocalFree for its returned allocation.
	return result, nil
}
