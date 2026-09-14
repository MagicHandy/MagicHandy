package accounts

import (
	"strings"
	"testing"
)

func TestSessionReferenceNeverAcceptsItsOwnDigestAsACredential(t *testing.T) {
	token := strings.Repeat("synthetic-session", 3)
	key := hashSessionToken(token)
	if !MatchesSessionKey(token, key) {
		t.Fatal("original token did not match")
	}
	for _, candidate := range []string{"", key, strings.Repeat("x", 129), strings.Repeat("y", 48)} {
		if MatchesSessionKey(candidate, key) {
			t.Fatal("non-credential matched private session identity")
		}
	}
}
