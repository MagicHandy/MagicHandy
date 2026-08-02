//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package httpapi

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureSetupProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killSetupProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return command.Process.Kill()
}
