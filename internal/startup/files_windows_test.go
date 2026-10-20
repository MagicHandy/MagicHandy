//go:build windows

package startup

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDocumentReplacementRetriesOnlyTransientWindowsDenial(t *testing.T) {
	for _, code := range []error{windows.ERROR_SHARING_VIOLATION, windows.ERROR_ACCESS_DENIED, windows.ERROR_FILE_NOT_FOUND} {
		calls := 0
		err := replaceDocumentUsing(func() error {
			calls++
			if calls < 3 {
				return &os.LinkError{Op: "rename", Old: "temporary", New: "current", Err: code}
			}
			return nil
		})
		if code == windows.ERROR_FILE_NOT_FOUND {
			if calls != 1 || !errors.Is(err, code) {
				t.Fatalf("unexpected retry for permanent failure: calls=%d err=%v", calls, err)
			}
		} else if err != nil || calls != 3 {
			t.Fatalf("transient denial did not recover: calls=%d err=%v", calls, err)
		}
	}
}

func TestDocumentReplacementRetriesAreBounded(t *testing.T) {
	calls := 0
	err := replaceDocumentUsing(func() error {
		calls++
		return windows.ERROR_SHARING_VIOLATION
	})
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) || calls != 5 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
