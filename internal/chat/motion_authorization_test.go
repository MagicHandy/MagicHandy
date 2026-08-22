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
		"start smooth motion with varied strokes without jitter",
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

func TestDirectPartnerActionCommandsAuthorizeOnlyClearStarts(t *testing.T) {
	for _, message := range []string{
		"Fuck me",
		"Please fuck me",
		"Could you suck me?",
		"I want you to kiss it",
		"Fuck me harder",
		"Fuck me and talk to me",
		"Stroke me gently",
		"Jerk me off",
		"Ride me however you want",
		"Suck me",
		"Suck it gently",
		"kiss it",
		"Lick it slowly",
	} {
		if !userAuthorizesMotion(message, MotionActionStart) {
			t.Errorf("direct partner-action start was refused: %q", message)
		}
		if !userAuthorizesMotion(message, MotionActionTarget) {
			t.Errorf("direct partner-action target was refused: %q", message)
		}
	}

	for _, message := range []string{
		"Well, fuck me",
		"Fuck me, that's funny",
		"They said fuck me in the story",
		"Tell me what fuck me means",
		"Say fuck me",
		"Tell me to say suck me",
		"They wrote kiss it in the story",
		"Don't fuck me",
		"I do not want you to stroke me",
		"Should I say fuck me?",
	} {
		if userAuthorizesMotion(message, MotionActionStart) ||
			userAuthorizesMotion(message, MotionActionTarget) {
			t.Errorf("ambiguous, quoted, or refused partner wording authorized motion: %q", message)
		}
	}
}

// Area requests only ever re-aim motion that is already running, and being
// refused is indistinguishable from being ignored. Before this list, only the
// exact phrase "focus on the tip" behind a directive prefix was recognized, so
// every other way of saying the same thing was silently dropped.
func TestAreaRequestsAuthorizeATargetChange(t *testing.T) {
	for _, message := range []string{
		"focus on the tip",
		"focus on my tip",
		"just the tip",
		"only the tip please",
		"stay near the top",
		"keep it shallow",
		"work the base",
		"concentrate on the shaft",
		"stick to the middle",
		"move to the base",
		"back to the full range",
		"use the whole stroke again",
		"enfócate en la punta",
		"solo la punta",
		"foque na ponta",
		"só a ponta",
		"集中在尖端",
		"先端だけ",
	} {
		if !userAuthorizesMotion(message, MotionActionTarget) {
			t.Errorf("area request was refused: %q", message)
		}
	}
}

// Naming a zone is not the same as asking for it, and it must never be a way
// around the start gate.
func TestAreaWordsDoNotStartMotionOrSurviveARefusal(t *testing.T) {
	for _, message := range []string{
		"just the tip",
		"focus on the tip",
		"stay near the top",
	} {
		if userAuthorizesMotion(message, MotionActionStart) {
			t.Errorf("area wording started motion: %q", message)
		}
	}
	for _, message := range []string{
		"don't focus on the tip",
		"is it safe to focus on the tip?",
		"what happens if you focus on the tip?",
		"tell me about the tip",
		"stop focusing on the tip",
	} {
		if userAuthorizesMotion(message, MotionActionTarget) {
			t.Errorf("refused or conversational area wording authorized a target: %q", message)
		}
	}
}
