package llm

// ProviderProgress carries timing and counters only. Hidden reasoning and
// prompt contents are deliberately excluded from diagnostics and callbacks.
type ProviderProgress struct {
	Activity         bool
	LoadMillis       int64
	PromptEvalMillis int64
	PromptTokens     int
	GeneratedTokens  int
}

func reportProgress(callbacks []func(ProviderProgress), progress ProviderProgress) {
	for _, callback := range callbacks {
		if callback != nil {
			callback(progress)
		}
	}
}
