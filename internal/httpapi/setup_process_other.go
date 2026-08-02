//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package httpapi

import (
	"os"
	"os/exec"
)

func configureSetupProcess(_ *exec.Cmd) {}

func killSetupProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
