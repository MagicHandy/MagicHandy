// Package updatecheck compares the running build with MagicHandy's latest
// stable GitHub release. It only discovers releases; installing one remains an
// explicit user action through the release page.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultReleaseAPIURL is GitHub's latest stable release endpoint. Drafts
	// and prereleases are intentionally outside the automatic update channel.
	DefaultReleaseAPIURL = "https://api.github.com/repos/MagicHandy/MagicHandy/releases/latest"
	defaultReleasePage   = "https://github.com/MagicHandy/MagicHandy/releases/tag/"
	defaultCacheTTL      = 6 * time.Hour
	defaultFailureTTL    = 15 * time.Minute
	maxResponseBytes     = 2 << 20
)

// State describes the result without making network failure look like an app
// failure.
type State string

const (
	// StateAvailable means a newer stable release exists.
	StateAvailable State = "available"
	// StateCurrent means the running version is at least as new as the latest stable release.
	StateCurrent State = "current"
	// StateDevelopment means the running build has no comparable semantic version.
	StateDevelopment State = "development"
	// StateNoRelease means the repository has no published stable release.
	StateNoRelease State = "no_release"
	// StateError means the release check could not produce a result.
	StateError State = "error"
)

// Release is the small, trusted subset of GitHub release data the UI needs.
type Release struct {
	Version     string    `json:"version"`
	Tag         string    `json:"tag"`
	Name        string    `json:"name,omitempty"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at,omitempty"`
}

// Status is returned by the local API for both startup and manual checks.
type Status struct {
	State          State     `json:"state"`
	CurrentVersion string    `json:"current_version"`
	Latest         *Release  `json:"latest,omitempty"`
	CheckedAt      time.Time `json:"checked_at,omitempty"`
	Stale          bool      `json:"stale,omitempty"`
	Message        string    `json:"message,omitempty"`
}

// Options configure a Checker. Endpoint, Clock, and CacheTTL are injectable so
// tests never contact GitHub.
type Options struct {
	CurrentVersion string
	Endpoint       string
	HTTPClient     *http.Client
	Clock          func() time.Time
	CacheTTL       time.Duration
}

// Checker serializes release requests and keeps their ETag-backed result in
// memory. A browser refresh therefore does not spend another unauthenticated
// GitHub API request.
type Checker struct {
	currentVersion string
	endpoint       string
	client         *http.Client
	clock          func() time.Time
	cacheTTL       time.Duration

	mu     sync.Mutex
	cached Status
	etag   string
	// lastAttempt throttles advisory retries after GitHub is unavailable. A
	// manual refresh still bypasses this delay.
	lastAttempt time.Time
}

// New constructs a release checker with conservative network defaults.
func New(options Options) *Checker {
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = DefaultReleaseAPIURL
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	cacheTTL := options.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	return &Checker{
		currentVersion: strings.TrimSpace(options.CurrentVersion),
		endpoint:       endpoint,
		client:         client,
		clock:          clock,
		cacheTTL:       cacheTTL,
	}
}

// Check returns a cached result unless refresh is requested or the cache has
// expired. A failed refresh keeps the last successful result, marked stale.
func (c *Checker) Check(ctx context.Context, refresh bool) Status {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock().UTC()
	if !refresh {
		if !c.cached.CheckedAt.IsZero() && !c.cached.Stale && c.cached.State != StateError && now.Sub(c.cached.CheckedAt) < c.cacheTTL {
			return c.cached
		}
		if !c.lastAttempt.IsZero() && now.Sub(c.lastAttempt) < defaultFailureTTL {
			return c.cached
		}
	}

	c.lastAttempt = now
	status, etag, notModified, err := c.fetch(ctx, now)
	if err != nil {
		if !c.cached.CheckedAt.IsZero() && c.cached.State != StateError {
			c.cached.Stale = true
			c.cached.Message = "GitHub Releases could not be refreshed. Showing the last successful result."
			return c.cached
		}
		c.cached = Status{
			State:          StateError,
			CurrentVersion: c.currentVersion,
			CheckedAt:      now,
			Message:        err.Error(),
		}
		return c.cached
	}
	if notModified {
		c.cached.CheckedAt = now
		c.cached.Stale = false
		c.cached.Message = ""
		return c.cached
	}
	c.cached = status
	c.etag = etag
	return c.cached
}

func (c *Checker) fetch(ctx context.Context, checkedAt time.Time) (Status, string, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return Status{}, "", false, errors.New("the release check is not configured correctly")
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "MagicHandy/"+displayVersion(c.currentVersion))
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	if c.etag != "" {
		request.Header.Set("If-None-Match", c.etag)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return Status{}, "", false, errors.New("GitHub Releases could not be reached")
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode == http.StatusNotModified {
		if c.cached.CheckedAt.IsZero() {
			return Status{}, "", false, errors.New("GitHub returned an empty cached release response")
		}
		return Status{}, response.Header.Get("ETag"), true, nil
	}
	if response.StatusCode == http.StatusNotFound {
		return Status{
			State:          StateNoRelease,
			CurrentVersion: c.currentVersion,
			CheckedAt:      checkedAt,
		}, response.Header.Get("ETag"), false, nil
	}
	if response.StatusCode != http.StatusOK {
		return Status{}, "", false, fmt.Errorf("GitHub Releases returned HTTP %d", response.StatusCode)
	}

	var payload struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		PublishedAt string `json:"published_at"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return Status{}, "", false, errors.New("GitHub returned an invalid release response")
	}
	status, err := compareRelease(c.currentVersion, payload.TagName, payload.Name, payload.PublishedAt, checkedAt)
	if err != nil {
		return Status{}, "", false, err
	}
	return status, response.Header.Get("ETag"), false, nil
}

func compareRelease(currentVersion, tag, name, published string, checkedAt time.Time) (Status, error) {
	latestVersion, ok := parseVersion(tag)
	if !ok {
		return Status{}, errors.New("the latest GitHub release does not use a semantic version tag")
	}
	release := &Release{
		Version: latestVersion.String(),
		Tag:     strings.TrimSpace(tag),
		Name:    trimText(name, 160),
		URL:     defaultReleasePage + url.PathEscape(strings.TrimSpace(tag)),
	}
	if timestamp, err := time.Parse(time.RFC3339, published); err == nil {
		release.PublishedAt = timestamp
	}
	status := Status{
		CurrentVersion: currentVersion,
		Latest:         release,
		CheckedAt:      checkedAt,
	}
	current, ok := parseVersion(currentVersion)
	if !ok {
		status.State = StateDevelopment
		return status, nil
	}
	if latestVersion.Compare(current) > 0 {
		status.State = StateAvailable
	} else {
		status.State = StateCurrent
	}
	return status, nil
}

func displayVersion(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "dev"
}

func trimText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
