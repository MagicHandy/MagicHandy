package httpapi

import (
	"crypto/rand"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
)

type coyoteCandidate struct {
	address string
	expires time.Time
}

type coyoteDiscoveryChoice struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	RSSI           int    `json:"rssi"`
	Identification string `json:"identification"`
}

var errCoyoteDiscoveryBusy = errors.New("bluetooth discovery is in progress; wait for the list before connecting")

func (c *coyoteRuntime) takeCandidate(id string) (string, error) {
	// Caller holds discoveryMu until Connect finishes, reserving this operation.
	if c.discovering {
		return "", errCoyoteDiscoveryBusy
	}
	if id == "" {
		return "", nil
	}
	candidate, ok := c.candidates[id]
	delete(c.candidates, id)
	if !ok || time.Now().After(candidate.expires) {
		return "", errors.New("device selection expired; find nearby devices again")
	}
	return candidate.address, nil
}

func (s *Server) handleCoyoteDiscover(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errCoyoteHostOnly)
		return
	}
	c := &s.coyote
	c.discoveryMu.Lock()
	if c.discovering {
		c.discoveryMu.Unlock()
		writeError(w, http.StatusConflict, errCoyoteDiscoveryBusy)
		return
	}
	if status := c.owner.Status(); status.Connected || status.Connecting {
		c.discoveryMu.Unlock()
		writeError(w, http.StatusConflict, errors.New("disconnect the Coyote before finding another device"))
		return
	}
	if c.discoverer == nil {
		c.discoveryMu.Unlock()
		writeError(w, http.StatusServiceUnavailable, errors.New("nearby device discovery is unavailable"))
		return
	}
	c.discovering = true
	c.candidates = nil
	c.discoveryMu.Unlock()
	defer func() { c.discoveryMu.Lock(); c.discovering = false; c.discoveryMu.Unlock() }()
	candidates, err := c.discoverer.Discover(r.Context(), 8*time.Second)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	slices.SortFunc(candidates, func(a, b ble.Candidate) int { return b.RSSI - a.RSSI })
	settings, _ := s.store.Snapshot()
	profile, _ := ble.LookupProfile(ble.ProfileCoyote)
	choices := make([]coyoteDiscoveryChoice, 0, min(len(candidates), 64))
	c.discoveryMu.Lock()
	c.candidates = make(map[string]coyoteCandidate)
	for _, candidate := range candidates[:min(len(candidates), 64)] {
		if candidate.Address == "" {
			continue
		}
		id := rand.Text()
		c.candidates[id] = coyoteCandidate{address: candidate.Address, expires: time.Now().Add(2 * time.Minute)}
		identification := "possible"
		if candidate.Address == settings.Estim.RememberedAddress {
			identification = "remembered"
		} else if profile.MatchesName(candidate.Name) {
			identification = "name_match"
		}
		choices = append(choices, coyoteDiscoveryChoice{ID: id, Name: candidate.Name, RSSI: candidate.RSSI, Identification: identification})
	}
	c.discoveryMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"candidates": choices})
}
