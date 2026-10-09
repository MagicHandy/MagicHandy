package openaiauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"strings"
	"time"
)

type verifiedIdentity struct{ Subject, Email string }
type identityHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}
type identityKey struct {
	KeyID     string `json:"kid"`
	Type      string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	N         string `json:"n"`
	E         string `json:"e"`
}
type identityClaims struct {
	Issuer          string          `json:"iss"`
	Subject         string          `json:"sub"`
	Audience        json.RawMessage `json:"aud"`
	Expires         int64           `json:"exp"`
	IssuedAt        int64           `json:"iat"`
	NotBefore       int64           `json:"nbf"`
	Nonce           string          `json:"nonce"`
	Email           string          `json:"email"`
	AuthorizedParty string          `json:"azp"`
}

var errInvalidIdentity = errors.New("OpenAI identity token could not be verified")

func (m *Manager) verifyIDToken(ctx context.Context, document discovery, token, clientID, nonce string) (verifiedIdentity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(token) > 32768 {
		return verifiedIdentity{}, errInvalidIdentity
	}
	var header identityHeader
	if decodeIdentityPart(parts[0], &header) != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return verifiedIdentity{}, errInvalidIdentity
	}
	public, err := m.identityPublicKey(ctx, document.JWKSURI, header.KeyID)
	if err != nil {
		return verifiedIdentity{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return verifiedIdentity{}, errInvalidIdentity
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(public, crypto.SHA256, hash[:], signature) != nil {
		return verifiedIdentity{}, errInvalidIdentity
	}
	var claims identityClaims
	if decodeIdentityPart(parts[1], &claims) != nil || !claims.valid(document.Issuer, clientID, nonce, time.Now()) {
		return verifiedIdentity{}, errInvalidIdentity
	}
	return verifiedIdentity{Subject: claims.Subject, Email: claims.Email}, nil
}
func decodeIdentityPart(part string, target any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, target)
}
func (m *Manager) identityPublicKey(ctx context.Context, endpoint, keyID string) (*rsa.PublicKey, error) {
	var keys struct {
		Keys []identityKey `json:"keys"`
	}
	if err := m.getJSON(ctx, endpoint, &keys); err != nil {
		return nil, err
	}
	for _, key := range keys.Keys {
		if key.KeyID != keyID || key.Type != "RSA" || (key.Use != "" && key.Use != "sig") || (key.Algorithm != "" && key.Algorithm != "RS256") {
			continue
		}
		if public := key.publicKey(); public != nil {
			return public, nil
		}
	}
	return nil, errInvalidIdentity
}
func (key identityKey) publicKey() *rsa.PublicKey {
	n, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil || len(n) < 256 || len(n) > 1024 {
		return nil
	}
	modulus := new(big.Int).SetBytes(n)
	if modulus.BitLen() < 2048 {
		return nil
	}
	e, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil || len(e) == 0 || len(e) > 4 {
		return nil
	}
	exponent := new(big.Int).SetBytes(e).Int64()
	if exponent < 3 || exponent > 2147483647 || exponent%2 == 0 {
		return nil
	}
	return &rsa.PublicKey{N: modulus, E: int(exponent)}
}
func (claims identityClaims) valid(issuer, clientID, nonce string, now time.Time) bool {
	if claims.Issuer != issuer || claims.Subject == "" || claims.Expires <= now.Unix() || claims.IssuedAt <= 0 || claims.IssuedAt > now.Add(30*time.Second).Unix() || claims.NotBefore > now.Add(30*time.Second).Unix() {
		return false
	}
	if nonce != "" && claims.Nonce != nonce {
		return false
	}
	var audience string
	var audiences []string
	if json.Unmarshal(claims.Audience, &audience) == nil {
		audiences = []string{audience}
	} else if json.Unmarshal(claims.Audience, &audiences) != nil {
		return false
	}
	return slices.Contains(audiences, clientID) && (len(audiences) <= 1 || claims.AuthorizedParty == clientID) && (claims.AuthorizedParty == "" || claims.AuthorizedParty == clientID)
}
