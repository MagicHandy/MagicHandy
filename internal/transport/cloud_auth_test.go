package transport

import (
	"errors"
	"strings"
	"testing"
)

func TestCloudAuthAcceptsDocumentedConnectionKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "five characters", key: "aB3dE"},
		{name: "six characters", key: "aB3dE6"},
		{name: "seven characters", key: "aB3dE67"},
		{name: "eight characters", key: "aB3dE678"},
		{name: "32 characters", key: strings.Repeat("aB3d", 8)},
		{name: "64 characters", key: strings.Repeat("aB3d", 16)},
		{name: "pasted outer whitespace", key: " \t\naB3dE\r\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			auth, err := BuildCloudAuthMetadata(validCloudPrerequisites(func(p *CloudPrerequisites) {
				p.ConnectionKey = test.key
			}))
			if err != nil {
				t.Fatalf("documented connection key was rejected: %v", err)
			}
			if auth.ConnectionKey != strings.TrimSpace(test.key) || !auth.ConnectionKeySet {
				t.Fatal("connection key was not preserved after trimming outer whitespace")
			}
		})
	}
}

func TestCloudAuthRejectsMalformedConnectionKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "four characters", key: "aB3d"},
		{name: "65 characters", key: strings.Repeat("a", 65)},
		{name: "internal space", key: "aB3 dE"},
		{name: "internal tab", key: "aB3\tdE"},
		{name: "internal newline", key: "aB3\ndE"},
		{name: "punctuation", key: "aB3-dE"},
		{name: "query delimiter", key: "aB3&dE"},
		{name: "non ASCII letters", key: "aB3dé"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildCloudAuthMetadata(validCloudPrerequisites(func(p *CloudPrerequisites) {
				p.ConnectionKey = test.key
			}))
			var unavailable HSPUnavailableError
			if !errors.As(err, &unavailable) || unavailable.Code != "malformed_connection_key" ||
				unavailable.Field != "connection_key" || !unavailable.NoFallback {
				t.Fatalf("expected fail-closed connection-key validation, got %v", err)
			}
			if strings.Contains(err.Error(), test.key) {
				t.Fatal("validation error disclosed the connection key")
			}
		})
	}
}
