//go:build !windows

package llm

import "os/exec"

type managedLlamaProcessLifetime interface {
	Close() error
}

func configureManagedLlamaProcess(_ *exec.Cmd) {}

func attachManagedLlamaProcess(_ *exec.Cmd) (managedLlamaProcessLifetime, error) {
	return nil, nil
}
