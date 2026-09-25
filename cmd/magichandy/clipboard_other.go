//go:build !windows

package main

// clipboardCopier returns nil, which hides the copy key; the launch console
// runs only on Windows.
func clipboardCopier(string) func() error { return nil }
