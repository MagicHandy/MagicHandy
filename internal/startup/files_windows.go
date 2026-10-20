//go:build windows

package startup

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func replaceDocument(old, next string) error {
	return replaceDocumentUsing(func() error { return os.Rename(old, next) })
}

// Antivirus and concurrent readers can briefly deny atomic replacement. Keep
// the private write atomic and bounded; permanent failures still reach callers.
func replaceDocumentUsing(rename func() error) error {
	for attempt := 0; ; attempt++ {
		err := rename()
		if err == nil || attempt == 4 || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			return err
		}
		time.Sleep(time.Duration(1<<attempt) * 10 * time.Millisecond)
	}
}
