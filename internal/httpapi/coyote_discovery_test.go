package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

type fixedCoyoteDiscoverer []ble.Candidate

func (d fixedCoyoteDiscoverer) Discover(context.Context, time.Duration) ([]ble.Candidate, error) {
	return append([]ble.Candidate(nil), d...), nil
}

func TestCoyoteDiscoveryIsPrivateRankedAndRequiresSelection(t *testing.T) {
	s, connector := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	s.coyote.discoverer = fixedCoyoteDiscoverer{
		{Address: "private-named", Name: "47L121000", RSSI: -60},
		{Address: "private-anonymous", RSSI: -24},
		{Address: "private-remembered", RSSI: -42},
	}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Estim.RememberedAddress = "private-remembered"
		return settings
	})
	request := withController(httptest.NewRequest(http.MethodPost, "/api/transport/coyote/discover", strings.NewReader(`{}`)))
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("discovery = %d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "private-") {
		t.Fatal("discovery exposed a hardware address")
	}
	if connector.LastLink() != nil {
		t.Fatal("scanning opened a device connection")
	}
	var response struct {
		Candidates []coyoteDiscoveryChoice `json:"candidates"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Candidates) != 3 || response.Candidates[0].RSSI != -24 || response.Candidates[0].Identification != "possible" || response.Candidates[1].Identification != "remembered" || response.Candidates[2].Identification != "name_match" {
		t.Fatalf("unexpected ranking or identity: %+v", response)
	}
	s.coyote.discoveryMu.Lock()
	defer s.coyote.discoveryMu.Unlock()
	id := response.Candidates[0].ID
	if address, err := s.coyote.takeCandidate(id); err != nil || address != "private-anonymous" {
		t.Fatal("selected ID did not resolve to its candidate")
	}
	if _, err := s.coyote.takeCandidate(id); err == nil {
		t.Fatal("consumed selection remained valid")
	}
	s.coyote.candidates["expired"] = coyoteCandidate{address: "private-expired", expires: time.Now().Add(-time.Second)}
	if _, err := s.coyote.takeCandidate("expired"); err == nil {
		t.Fatal("expired selection was accepted")
	}
}

func TestCoyoteInvalidSelectionCannotFallBackToAutomaticConnect(t *testing.T) {
	s, connector := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	code, _ := coyoteRequest(t, s, http.MethodPost, "/api/transport/coyote/connect", `{"candidate_id":"expired"}`, true)
	if code != http.StatusConflict || connector.LastLink() != nil {
		t.Fatalf("invalid selection opened a connection: %d", code)
	}
}
