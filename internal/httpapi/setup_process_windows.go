//go:build windows

package httpapi

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func configureSetupProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func killSetupProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	taskkill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F") // #nosec G204 -- PID is numeric and app-owned.
	taskkill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := taskkill.Run(); err == nil {
		return nil
	}
	err := command.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
