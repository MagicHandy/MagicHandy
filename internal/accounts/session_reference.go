package accounts

import "crypto/subtle"

// MatchesSessionKey recognizes a token for an already-held server identity.
// It performs no authentication, expiry, revocation or permission check. The
// HTTP edge uses it only for scheduling; admission still resolves the session.
func MatchesSessionKey(token, key string) bool {
	if len(token) < 32 || len(token) > 128 || len(key) != 64 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashSessionToken(token)), []byte(key)) == 1
}
