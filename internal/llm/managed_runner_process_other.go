//go:build !windows

package llm

import "errors"

// FindManagedRunnerProcesses is currently implemented for Windows, where the
// managed desktop installer and duplicate-runtime recovery workflow are used.
func FindManagedRunnerProcesses(_ string, _ int) ([]ManagedRunnerProcess, error) {
	return nil, nil
}

// TerminateManagedRunnerProcess rejects the recovery action outside the
// currently supported Windows managed desktop runtime.
func TerminateManagedRunnerProcess(_ string, _ int) error {
	return errors.New("managed runner process termination is only supported on Windows")
}
