//go:build windows

package llm

import (
	"os"
	"os/exec"
	"strconv"
)

func cancelManagedBuildProcess(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	taskkill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F") // #nosec G204 -- PID is numeric and app-owned.
	if err := taskkill.Run(); err == nil {
		return nil
	}
	return command.Process.Kill()
}
