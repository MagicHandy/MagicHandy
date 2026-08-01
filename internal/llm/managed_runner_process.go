package llm

// ManagedRunnerProcess identifies an already-running copy of the exact
// executable configured for MagicHandy's managed llama.cpp runtime.
type ManagedRunnerProcess struct {
	PID        int    `json:"pid"`
	Executable string `json:"executable"`
}
