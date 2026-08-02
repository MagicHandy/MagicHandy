//go:build !windows

package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openSystemBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url) // #nosec G204 -- URL is constructed from the validated loopback listener.
	case "linux", "freebsd", "openbsd", "netbsd":
		command = exec.Command("xdg-open", url) // #nosec G204 -- URL is constructed from the validated loopback listener.
	default:
		return fmt.Errorf("opening a browser is unsupported on %s", runtime.GOOS)
	}
	return command.Run()
}
