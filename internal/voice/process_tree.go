package voice

import (
	"os/exec"

	"github.com/mapledaemon/MagicHandy/internal/processtree"
)

func configureWorkerProcess(command *exec.Cmd) {
	processtree.Configure(command)
}

func killWorkerProcess(command *exec.Cmd) error {
	return processtree.Kill(command)
}
