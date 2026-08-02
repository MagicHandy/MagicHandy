//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func openSystemBrowser(url string) error {
	command := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url) // #nosec G204 -- URL is constructed from the validated loopback listener.
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Run()
}
