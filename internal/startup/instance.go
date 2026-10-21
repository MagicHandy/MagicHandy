package startup

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrRunning means another process holds this profile's operating-system lock.
var ErrRunning = errors.New("MagicHandy is already starting or running for this data directory")

const identityPath = "/startup/identity"

// Instance holds the profile lock until all application workers have stopped.
// The lock file is never unlinked: replacing it could allow two lock owners.
type Instance struct {
	file *os.File
	root string
	mu   sync.RWMutex
	info instanceRecord
}

type instanceRecord struct {
	Version            int       `json:"schema_version"`
	PID                int       `json:"pid"`
	ID                 string    `json:"instance_id"`
	Token              string    `json:"token"`
	URL                string    `json:"url"`
	RemoteURL          string    `json:"remote_url,omitempty"`
	PreferredURL       string    `json:"preferred_url,omitempty"`
	PreferredRemoteURL string    `json:"preferred_remote_url,omitempty"`
	StartedAt          time.Time `json:"started_at"`
}

// Acquire creates no datastore or workers. OS locks are released on crashes.
func Acquire(dataDir string) (*Instance, error) {
	root, err := prepareDirectory(dataDir)
	if err != nil {
		return nil, fmt.Errorf("prepare startup storage: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(root, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600) // #nosec G304 -- fixed profile lock file.
	if err != nil {
		return nil, err
	}
	if err := tryLock(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	// Never trust a crash's stale endpoint after acquiring a new lock.
	_ = os.Remove(filepath.Join(root, "instance.json"))
	return &Instance{file: file, root: root, info: instanceRecord{
		Version: 1, PID: os.Getpid(), ID: randomHex(), Token: randomHex(), StartedAt: time.Now().UTC(),
	}}, nil
}

func randomHex() string {
	var value [32]byte
	_, _ = rand.Read(value[:]) // crypto/rand.Read never returns an error.
	return hex.EncodeToString(value[:])
}

// Publish exposes only prepared endpoints. A caller also has to complete a
// fresh challenge through the serving socket before accepting the record.
func (i *Instance) Publish(appURL, remoteURL string) error {
	return i.PublishWithPreference(appURL, remoteURL, appURL, remoteURL)
}

// PublishWithPreference records user intent separately from the bound endpoint.
func (i *Instance) PublishWithPreference(appURL, remoteURL, preferredURL, preferredRemoteURL string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.info.URL, i.info.RemoteURL = appURL, remoteURL
	i.info.PreferredURL = preferredURL
	i.info.PreferredRemoteURL = preferredRemoteURL
	return writeDocument(i.root, "instance.json", i.info)
}

// Close removes readiness before releasing the profile lock. Call after teardown.
func (i *Instance) Close() {
	_ = os.Remove(filepath.Join(i.root, "instance.json"))
	_ = i.file.Close()
}

func proof(token, challenge string) string {
	key, _ := hex.DecodeString(token)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(challenge))
	return hex.EncodeToString(mac.Sum(nil))
}

// Handler proves the local process's identity without exposing the private
// token to a potentially unrelated service occupying a stale endpoint. The
// deliberately public response contains no account, settings or device state.
func (i *Instance) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != identityPath {
			next.ServeHTTP(w, r)
			return
		}
		i.mu.RLock()
		info := i.info
		i.mu.RUnlock()
		origin, err := localOrigin(info.URL)
		peer, _, _ := net.SplitHostPort(r.RemoteAddr)
		challenge := r.URL.Query().Get("challenge")
		decoded, decodeErr := hex.DecodeString(challenge)
		if err != nil || r.Method != http.MethodGet || r.Host != origin.Host || !net.ParseIP(peer).IsLoopback() || decodeErr != nil || len(decoded) != 32 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(map[string]string{"instance_id": info.ID, "proof": proof(info.Token, challenge)})
	})
}

func localOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Port() == "" || !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, errors.New("existing instance is not an automatically discoverable local HTTP app")
	}
	return u, nil
}
