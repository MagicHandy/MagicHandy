//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

// Package processtree owns platform-specific process-group lifecycle helpers.
package processtree

import (
	"os"
	"os/exec"
)

// Configure leaves commands unchanged on platforms without process groups.
func Configure(_ *exec.Cmd) {}

// Attach returns a handle that terminates command on close.
func Attach(command *exec.Cmd) (*Handle, error) {
	if command == nil || command.Process == nil {
		return nil, os.ErrProcessDone
	}
	return newHandle(command.Process.Kill), nil
}

// Kill terminates command on platforms without process-tree support.
func Kill(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
