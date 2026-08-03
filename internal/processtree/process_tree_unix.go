//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

// Package processtree owns platform-specific process-group lifecycle helpers.
package processtree

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Configure gives command a process group that can be stopped as one unit.
func Configure(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Attach returns a handle that terminates command's process group on close.
func Attach(command *exec.Cmd) (*Handle, error) {
	if command == nil || command.Process == nil {
		return nil, os.ErrProcessDone
	}
	pid := command.Process.Pid
	return newHandle(func() error {
		err := syscall.Kill(-pid, syscall.SIGKILL)
		if err == nil || errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return command.Process.Kill()
	}), nil
}

// Kill terminates command and every process in its process group.
func Kill(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return command.Process.Kill()
}
