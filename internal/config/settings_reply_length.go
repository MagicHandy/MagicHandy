package config

// Reply lengths steer how long chat replies run. They shape prose only; the
// motion contract and every motion gate are identical at every length.
const (
	// LLMReplyLengthShort asks for one or two short sentences per reply.
	LLMReplyLengthShort = "short"
	// LLMReplyLengthBalanced composes no length instruction: the prompts'
	// own pacing, as tuned.
	LLMReplyLengthBalanced = "balanced"
	// LLMReplyLengthDetailed allows three to five sentences when the moment
	// calls for it, with room in the output budget to finish them.
	LLMReplyLengthDetailed = "detailed"
	// DetailedReplyMinOutputTokens keeps a detailed reply and its motion JSON
	// from running into the output cap.
	DetailedReplyMinOutputTokens = 512
)

// LLMReplyLengths lists the reply lengths from shortest to longest.
func LLMReplyLengths() []string {
	return []string{LLMReplyLengthShort, LLMReplyLengthBalanced, LLMReplyLengthDetailed}
}

// ValidLLMReplyLength reports whether a reply length is one this build composes.
func ValidLLMReplyLength(length string) bool {
	return oneOf(length, LLMReplyLengths()...)
}

// ChatMaxOutputTokens is the output budget for one chat reply at the given
// reply length: the saved cap, raised for detailed replies so they can
// finish inside the JSON contract.
func (s LLMSettings) ChatMaxOutputTokens(replyLength string) int {
	if replyLength == LLMReplyLengthDetailed && s.MaxOutputTokens < DetailedReplyMinOutputTokens {
		return DetailedReplyMinOutputTokens
	}
	return s.MaxOutputTokens
}
