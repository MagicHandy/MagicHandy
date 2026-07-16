//go:build windows

package voice

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func configureWorkerProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func killWorkerProcess(command *exec.Cmd) error {
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
