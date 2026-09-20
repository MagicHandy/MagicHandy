package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

func TestGuidedNetworkPreparationRequiresPasswordAndSeparateSave(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	config := netaccess.Config{Mode: netaccess.DirectHTTPS, Scope: "lan", CertificateMode: netaccess.LocalCA, ListenAddress: "127.0.0.1:49717", PublicURL: "https://127.0.0.1:49717"}
	for _, password := range []string{"", "incorrect", "a long review passphrase"} {
		response := recoveryRequest(t, s, cookie, http.MethodPost, "/api/network/certificate", networkChange{Config: config, Password: password})
		want := http.StatusForbidden
		if password == "a long review passphrase" {
			want = http.StatusAccepted
		}
		if response.Code != want {
			t.Fatalf("prepare status=%d want=%d: %s", response.Code, want, response.Body.String())
		}
	}
	deadline := time.Now().Add(time.Second)
	for s.networkAutomation.Snapshot().State == "running" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.networkAutomation.Snapshot().State != "ready" {
		t.Fatal(s.networkAutomation.Snapshot())
	}
	saved, err := netaccess.Load(t.Context(), s.store.Datastore())
	if err != nil || saved != nil {
		t.Fatal("certificate preparation changed network configuration")
	}
	response := recoveryRequest(t, s, cookie, http.MethodGet, "/api/network/local-trust", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "BEGIN CERTIFICATE") || strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatal("trust download exposed non-public material")
	}
	response = recoveryRequest(t, s, cookie, http.MethodPut, "/api/network", networkChange{Config: config, Password: "a long review passphrase"})
	if response.Code != http.StatusOK {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	response = recoveryRequest(t, s, cookie, http.MethodGet, "/api/network", nil)
	var status struct {
		Saved   netaccess.Config `json:"saved"`
		Restart bool             `json:"restart_required"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Restart || status.Saved.CertificateMode != netaccess.LocalCA || status.Saved.TLSPrivateKey != "" {
		t.Fatalf("unexpected saved status: %+v", status)
	}
	response = recoveryRequest(t, s, cookie, http.MethodGet, "/api/network/report", nil)
	for _, secret := range []string{"PRIVATE KEY", "https-private", "acme-account", "127.0.0.1"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("connection report leaked certificate or host data")
		}
	}
}

func TestUnpreparedManagedConfigCannotEnableRemoteAccess(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	config := netaccess.Config{Mode: netaccess.DirectHTTPS, Scope: "lan", CertificateMode: netaccess.LocalCA, ListenAddress: "127.0.0.1:49717", PublicURL: "https://127.0.0.1:49717"}
	response := recoveryRequest(t, s, cookie, http.MethodPut, "/api/network", networkChange{Config: config, Password: "a long review passphrase"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("saved without certificate: %d %s", response.Code, response.Body.String())
	}
}
