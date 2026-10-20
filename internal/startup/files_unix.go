//go:build !windows

package startup

import "os"

func replaceDocument(old, next string) error { return os.Rename(old, next) }
