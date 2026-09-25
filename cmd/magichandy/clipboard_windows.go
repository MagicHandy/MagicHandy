//go:build windows

package main

import (
	"os/exec"
	"strings"
	"syscall"
)

// clipboardCopier copies the app link with the clip.exe that ships with
// Windows.
func clipboardCopier(link string) func() error {
	return func() error {
		command := exec.Command("clip.exe")
		command.Stdin = strings.NewReader(link)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return command.Run()
	}
}
