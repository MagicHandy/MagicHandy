package accounts

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashRoundTripAndUniqueSalt(t *testing.T) {
	password := "correct horse battery staple"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword first: %v", err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword second: %v", err)
	}
	if first == second {
		t.Fatal("password hashes reused a salt")
	}
	matched, err := VerifyPassword(password, first)
	if err != nil || !matched {
		t.Fatalf("VerifyPassword correct = (%t, %v), want true", matched, err)
	}
	matched, err = VerifyPassword("not the password", first)
	if err != nil || matched {
		t.Fatalf("VerifyPassword wrong = (%t, %v), want false", matched, err)
	}
}

func TestPasswordValidationBounds(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "seven ASCII characters", password: strings.Repeat("a", MinPasswordCharacters-1), wantErr: true},
		{name: "eight ASCII characters", password: strings.Repeat("a", MinPasswordCharacters)},
		{name: "eight multibyte characters", password: strings.Repeat("密", MinPasswordCharacters)},
		{name: "maximum bytes", password: strings.Repeat("a", MaxPasswordBytes)},
		{name: "over maximum bytes", password: strings.Repeat("a", MaxPasswordBytes+1), wantErr: true},
		{name: "invalid UTF-8", password: string([]byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8}), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidatePassword(test.password); (err != nil) != test.wantErr {
				t.Errorf("ValidatePassword(bytes=%d) = %v, wantErr %t", len(test.password), err, test.wantErr)
			}
		})
	}
}

func TestPasswordHashParserRejectsUnsafeParameters(t *testing.T) {
	valid, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	for _, encoded := range []string{
		"plain text",
		strings.Replace(valid, "m=19456", "m=999999999", 1),
		strings.Replace(valid, "t=2", "t=0", 1),
		strings.Replace(valid, "p=1", "p=99", 1),
	} {
		if _, err := VerifyPassword("correct horse battery staple", encoded); !errors.Is(err, errInvalidPasswordHash) {
			t.Errorf("VerifyPassword(%q) error = %v, want invalid hash", encoded, err)
		}
	}
}
