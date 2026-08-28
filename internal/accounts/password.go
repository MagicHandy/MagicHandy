package accounts

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// MinPasswordBytes is the minimum accepted UTF-8 password length.
	MinPasswordBytes = 12
	// MaxPasswordBytes bounds password-hashing work and request memory.
	MaxPasswordBytes = 1024

	passwordMemoryKiB = 19 * 1024
	passwordPasses    = 2
	passwordThreads   = 1
	passwordSaltBytes = 16
	passwordHashBytes = 32
)

var errInvalidPasswordHash = errors.New("invalid password hash")

// ErrInvalidPassword marks a password-policy failure without exposing a hash.
var ErrInvalidPassword = errors.New("invalid password")

type passwordParameters struct {
	memory  uint32
	passes  uint32
	threads uint8
	keyLen  uint32
}

var currentPasswordParameters = passwordParameters{
	memory:  passwordMemoryKiB,
	passes:  passwordPasses,
	threads: passwordThreads,
	keyLen:  passwordHashBytes,
}

// ValidatePassword applies only safety bounds. MagicHandy deliberately avoids
// composition rules that encourage predictable substitutions; callers should
// use a long, unique passphrase.
func ValidatePassword(password string) error {
	length := len([]byte(password))
	if length < MinPasswordBytes {
		return fmt.Errorf("%w: password must contain at least %d bytes", ErrInvalidPassword, MinPasswordBytes)
	}
	if length > MaxPasswordBytes {
		return fmt.Errorf("%w: password exceeds %d bytes", ErrInvalidPassword, MaxPasswordBytes)
	}
	return nil
}

// HashPassword returns a self-describing Argon2id PHC string. The parameters
// are stored with each hash so they can be raised later without invalidating
// existing accounts.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	derived := derivePassword([]byte(password), salt, currentPasswordParameters)
	return encodePasswordHash(currentPasswordParameters, salt, derived), nil
}

// VerifyPassword compares password to a bounded Argon2id PHC string. Database
// values are treated as untrusted: excessive cost values are rejected before
// they can consume attacker-controlled memory or CPU.
func VerifyPassword(password, encoded string) (bool, error) {
	parameters, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, err
	}
	actual := derivePassword([]byte(password), salt, parameters)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func derivePassword(password, salt []byte, parameters passwordParameters) []byte {
	return argon2.IDKey(password, salt, parameters.passes, parameters.memory, parameters.threads, parameters.keyLen)
}

func encodePasswordHash(parameters passwordParameters, salt, hash []byte) string {
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		parameters.memory,
		parameters.passes,
		parameters.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func parsePasswordHash(encoded string) (passwordParameters, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return passwordParameters{}, nil, nil, errInvalidPasswordHash
	}

	parameters, err := parsePasswordParameters(parts[3])
	if err != nil {
		return passwordParameters{}, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return passwordParameters{}, nil, nil, errInvalidPasswordHash
	}
	hash, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(hash) < 16 || len(hash) > 64 {
		return passwordParameters{}, nil, nil, errInvalidPasswordHash
	}
	parameters.keyLen = uint32(len(hash)) // #nosec G115 -- hash length is bounded to 64 immediately above.
	return parameters, salt, hash, nil
}

func parsePasswordParameters(encoded string) (passwordParameters, error) {
	parameters := passwordParameters{}
	values := strings.Split(encoded, ",")
	if len(values) != 3 {
		return passwordParameters{}, errInvalidPasswordHash
	}
	for _, value := range values {
		name, raw, ok := strings.Cut(value, "=")
		if !ok {
			return passwordParameters{}, errInvalidPasswordHash
		}
		parsed, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return passwordParameters{}, errInvalidPasswordHash
		}
		switch name {
		case "m":
			parameters.memory = uint32(parsed)
		case "t":
			parameters.passes = uint32(parsed)
		case "p":
			if parsed > 255 {
				return passwordParameters{}, errInvalidPasswordHash
			}
			parameters.threads = uint8(parsed)
		default:
			return passwordParameters{}, errInvalidPasswordHash
		}
	}
	if parameters.memory < 7*1024 || parameters.memory > 256*1024 ||
		parameters.passes == 0 || parameters.passes > 10 ||
		parameters.threads == 0 || parameters.threads > 16 {
		return passwordParameters{}, errInvalidPasswordHash
	}
	return parameters, nil
}
