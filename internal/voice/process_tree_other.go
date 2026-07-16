//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package voice

import (
	"os"
	"os/exec"
)

func configureWorkerProcess(_ *exec.Cmd) {}

func killWorkerProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
