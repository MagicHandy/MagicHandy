package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// credentialRevision fingerprints only the write-only voice credentials. A
// form that is replacing a key sends it back, so the save conflicts when
// another client changed a key, but not when an unrelated control (a motion
// slider, a voice preference) saved settings in the meantime. The salt is
// random per process, so the value reveals nothing about the keys and simply
// changes after a restart.
func (s *Server) credentialRevision(settings config.Settings) string {
	s.credentialSaltOnce.Do(func() {
		// crypto/rand.Read never returns an error as of Go 1.24.
		_, _ = rand.Read(s.credentialSalt[:])
	})
	hash := sha256.New()
	_, _ = hash.Write(s.credentialSalt[:])
	_, _ = hash.Write([]byte(settings.Voice.ElevenLabsAPIKey))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(settings.Voice.OpenAITTSAPIKey))
	return "c1-" + hex.EncodeToString(hash.Sum(nil)[:12])
}

// credentialRevisionMatches accepts the credential fingerprint, or the
// whole-settings revision that earlier clients send.
func (s *Server) credentialRevisionMatches(revision string, current config.Settings, loadedAt string) bool {
	if revision == "" {
		return false
	}
	return revision == loadedAt || revision == s.credentialRevision(current)
}
