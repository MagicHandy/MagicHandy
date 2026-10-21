package startup

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

var errNotDiscoverable = errors.New("this profile's endpoint is HTTPS or outside local HTTP discovery; open its configured URL or close the running instance before starting again")

// Existing is the public, verified part of a running instance record.
type Existing struct {
	PID                int    `json:"pid"`
	ID                 string `json:"instance_id"`
	URL                string `json:"url"`
	RemoteURL          string `json:"remote_url,omitempty"`
	PreferredURL       string `json:"preferred_url,omitempty"`
	PreferredRemoteURL string `json:"preferred_remote_url,omitempty"`
}

// FindExisting waits for an instance already holding the profile lock. Redirects
// and environment proxies are disabled; the token itself is never transmitted.
func FindExisting(ctx context.Context, dataDir string) (Existing, error) {
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if info, err := verifyExisting(ctx, dataDir, client); err == nil {
			return info, nil
		} else if errors.Is(err, errNotDiscoverable) {
			return Existing{}, err
		}
		select {
		case <-ctx.Done():
			return Existing{}, errors.New("another MagicHandy instance holds this profile but its local endpoint could not be verified; wait for startup or close that instance")
		case <-ticker.C:
		}
	}
}

func verifyExisting(ctx context.Context, dataDir string, client *http.Client) (Existing, error) {
	var record instanceRecord
	if err := readDocument(dataDir, "instance.json", &record); err != nil {
		return Existing{}, err
	}
	token, err := hex.DecodeString(record.Token)
	if err != nil || len(token) != 32 || len(record.ID) != 64 || record.Version != 1 || record.PID < 1 {
		return Existing{}, errors.New("invalid instance identity")
	}
	origin, err := localOrigin(record.URL)
	if err != nil {
		return Existing{}, errNotDiscoverable
	}
	challenge := randomHex()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.String()+identityPath+"?challenge="+challenge, nil)
	if err != nil {
		return Existing{}, err
	}
	response, err := client.Do(request) // #nosec G704 -- validated literal loopback HTTP origin; no redirects or proxies.
	if err != nil {
		return Existing{}, err
	}
	defer func() { _ = response.Body.Close() }()
	var answer struct {
		ID    string `json:"instance_id"`
		Proof string `json:"proof"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1024)).Decode(&answer) != nil {
		return Existing{}, errors.New("instance identity check failed")
	}
	expected, _ := hex.DecodeString(proof(record.Token, challenge))
	actual, _ := hex.DecodeString(answer.Proof)
	if answer.ID != record.ID || !hmac.Equal(expected, actual) {
		return Existing{}, errors.New("instance identity proof did not match")
	}
	return Existing{PID: record.PID, ID: record.ID, URL: record.URL, RemoteURL: record.RemoteURL,
		PreferredURL: record.PreferredURL, PreferredRemoteURL: record.PreferredRemoteURL}, nil
}
