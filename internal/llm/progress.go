package llm

// ProviderProgress carries timing and counters only. Hidden reasoning and
// prompt contents are deliberately excluded from diagnostics and callbacks.
type ProviderProgress struct {
	Activity         bool
	LoadMillis       int64
	PromptEvalMillis int64
	PromptTokens     int
	GeneratedTokens  int
	// ReasoningChars counts hidden reasoning characters in this update. The
	// text itself never leaves the provider; the count lets the model check
	// notice a model that reasons silently while thinking is off.
	ReasoningChars int
	// DecodeMillis and DecodeTokens time the generation phase when the
	// provider reports it, for a tokens-per-second figure.
	DecodeMillis int64
	DecodeTokens int
}

func reportProgress(callbacks []func(ProviderProgress), progress ProviderProgress) {
	for _, callback := range callbacks {
		if callback != nil {
			callback(progress)
		}
	}
}
