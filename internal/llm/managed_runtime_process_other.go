//go:build !windows

package llm

import (
	"os"
	"os/exec"
)

func cancelManagedBuildProcess(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
