package chat

import "testing"

// A question about moving contains the same verbs as a request to move, and a
// contracted negative ("didn't") refuses just as plainly as "do not". Neither
// may authorize the model to start the device: the safe failure for an
// ambiguous turn is a reply that only talks.
func TestQuestionsAndContractedNegativesNeverAuthorizeMotion(t *testing.T) {
	for _, message := range []string{
		"is it safe to start moving?",
		"is that safe to start moving?",
		"what happens if you start moving?",
		"what would happen if you start moving?",
		"should I start the motion?",
		"would it be bad to start moving?",
		"is it a good idea to start moving?",
		"may I start the motion?",
		"I didn't want you to start moving",
		"I didn't ask you to start moving",
		"you shouldn't start moving",
		"you can't start moving yet",
		"I never said to start moving",
		"¿es seguro empezar a mover?",
		"é seguro começar a mover?",
	} {
		if userAuthorizesMotion(message, MotionActionStart) {
			t.Errorf("start authorized by a question or negation: %q", message)
		}
		if userAuthorizesMotion(message, MotionActionTarget) {
			t.Errorf("target authorized by a question or negation: %q", message)
		}
	}
}

// The same gates must not swallow ordinary requests, or the model would stop
// responding to the wording users actually type.
func TestPlainRequestsStillAuthorizeMotion(t *testing.T) {
	for _, message := range []string{
		"start moving",
		"can you start moving?",
		"please start the motion",
		"begin moving now",
	} {
		if !userAuthorizesMotion(message, MotionActionStart) {
			t.Errorf("plain start request was refused: %q", message)
		}
	}
	for _, message := range []string{
		"faster",
		"go slower",
		"please change the pattern",
	} {
		if !userAuthorizesMotion(message, MotionActionTarget) {
			t.Errorf("plain adjustment request was refused: %q", message)
		}
	}
}
