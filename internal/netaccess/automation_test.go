package netaccess

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mholt/acmez/v3"
	"github.com/mholt/acmez/v3/acme"
)

const testTerms = "https://letsencrypt.org/documents/test-terms.pdf"

func managedPolicy(t *testing.T, mode string) *Policy {
	t.Helper()
	c := Config{Mode: DirectHTTPS, Scope: "lan", CertificateMode: mode, ListenAddress: "127.0.0.1:49717", PublicURL: "https://127.0.0.1:49717"}
	if mode == AutomaticPublic {
		c.Scope, c.PublicURL, c.AcceptedTerms = "public", "https://8.8.8.8", testTerms
	}
	p, err := Validate(c)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func awaitPreparation(t *testing.T, a *Automation) PreparationStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status := a.Snapshot()
		if status.State != "running" {
			return status
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("certificate job did not finish")
	return PreparationStatus{}
}

func TestLocalCertificateTrustAndPersistence(t *testing.T) {
	a := NewAutomation(t.TempDir())
	defer a.Close()
	p := managedPolicy(t, LocalCA)
	if a.ValidatePrepared(p) == nil {
		t.Fatal("missing certificate accepted")
	}
	if _, err := a.Prepare(p); err != nil {
		t.Fatal(err)
	}
	ready := awaitPreparation(t, a)
	if ready.State != "ready" {
		t.Fatalf("preparation: %+v", ready)
	}
	root, err := a.LocalTrustCertificate()
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(root)
	if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 || strings.Contains(string(root), "PRIVATE") {
		t.Fatal("trust export included something other than a public certificate")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := a.LoadManaged(p)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("protected transport")) }))
	server.Listener = tls.NewListener(server.Listener, provider.TLSConfig())
	server.Start()
	defer server.Close()
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get("https://" + server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.Status)
	}
	if _, err := provider.TLSConfig().GetCertificate(&tls.ClientHelloInfo{ServerName: "other.example"}); err == nil {
		t.Fatal("served certificate for unexpected identity")
	}
	if _, err := a.Prepare(p); err != nil {
		t.Fatal(err)
	}
	if a.Snapshot().Certificate.Fingerprint != ready.Certificate.Fingerprint {
		t.Fatal("reissued a fresh certificate")
	}
	if !provider.Status().Managed || provider.Status().RenewalDue {
		t.Fatal("fresh local certificate renewal status is wrong")
	}
}

func TestAutomaticCertificateConfigurationBoundaries(t *testing.T) {
	base := managedPolicy(t, AutomaticPublic).Config
	for name, change := range map[string]func(*Config){
		"private public IP": func(c *Config) { c.PublicURL = "https://192.168.1.2" },
		"CGNAT":             func(c *Config) { c.PublicURL = "https://100.64.1.2" },
		"special IP":        func(c *Config) { c.PublicURL = "https://203.0.113.3" },
		"wildcard bind":     func(c *Config) { c.ListenAddress = "0.0.0.0:443" },
		"external port":     func(c *Config) { c.PublicURL = "https://example.com:8443" },
		"no consent":        func(c *Config) { c.AcceptedTerms = "" },
		"wrong terms host":  func(c *Config) { c.AcceptedTerms = "https://example.com/documents/terms.pdf" },
		"file override":     func(c *Config) { c.TLSPrivateKey = "private.pem" },
		"public local CA":   func(c *Config) { c.CertificateMode = LocalCA },
	} {
		t.Run(name, func(t *testing.T) {
			c := base
			change(&c)
			if _, err := Validate(c); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
	for _, host := range []string{"example.com", "8.8.4.4", "[2606:4700:4700::1111]"} {
		c := base
		c.PublicURL = "https://" + host
		if _, err := Validate(c); err != nil {
			t.Fatalf("public identity %s: %v", host, err)
		}
	}
}

func TestLANScopeRejectsPublicPeers(t *testing.T) {
	p := managedPolicy(t, LocalCA)
	for _, peer := range []struct {
		ip      string
		allowed bool
	}{{"192.168.1.4", true}, {"127.0.0.1", true}, {"fd00::4", true}, {"8.8.8.8", false}} {
		r := httptest.NewRequest(http.MethodGet, p.Config.PublicURL+"/api/health", nil)
		r.RemoteAddr = net.JoinHostPort(peer.ip, "50000")
		_, err := p.Accept(r)
		if (err == nil) != peer.allowed {
			t.Fatalf("%s allowed=%v: %v", peer.ip, peer.allowed, err)
		}
	}
}

func TestChallengeHandshakeIsScopedAndTemporary(t *testing.T) {
	solver := &challengeSolver{host: "8.8.8.8"}
	challenge := acme.Challenge{Identifier: acme.Identifier{Type: "ip", Value: "8.8.8.8"}, KeyAuthorization: "test-token.test-thumbprint"}
	if err := solver.Present(context.Background(), challenge); err != nil {
		t.Fatal(err)
	}
	config := solver.config(nil)
	for _, hello := range []*tls.ClientHelloInfo{
		{ServerName: "8.8.8.8", SupportedProtos: []string{"h2"}},
		{ServerName: "8.8.8.8", SupportedProtos: []string{acmez.ACMETLS1Protocol}},
		{ServerName: "8.8.8.8.in-addr.arpa", SupportedProtos: []string{acmez.ACMETLS1Protocol, "h2"}},
	} {
		if _, err := config.GetConfigForClient(hello); err == nil {
			t.Fatal("accepted ordinary or unrelated validation handshake")
		}
	}
	hello := &tls.ClientHelloInfo{ServerName: "8.8.8.8.in-addr.arpa", SupportedProtos: []string{acmez.ACMETLS1Protocol}}
	if _, err := config.GetConfigForClient(hello); err != nil {
		t.Fatal(err)
	}
	if err := solver.CleanUp(context.Background(), challenge); err != nil {
		t.Fatal(err)
	}
	if _, err := config.GetConfigForClient(hello); err == nil {
		t.Fatal("served challenge after cleanup")
	}
	if got := challengeServerName("2001:db8::1"); !strings.HasPrefix(got, "1.0.0.0.") || !strings.HasSuffix(got, ".8.b.d.0.1.0.0.2.ip6.arpa") {
		t.Fatal(got)
	}
}

func TestCloseCancelsPreparationAndReleasesListener(t *testing.T) {
	a := NewAutomation(t.TempDir())
	p := managedPolicy(t, AutomaticPublic)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p.Config.ListenAddress = listener.Addr().String()
	_ = listener.Close()
	started := make(chan struct{})
	a.issue = func(ctx context.Context, _ *Policy, _ *challengeSolver) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, errors.New("certificate preparation was canceled")
	}
	if _, err := a.Prepare(p); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("issuer did not start")
	}
	a.Close()
	if a.Snapshot().State != "failed" {
		t.Fatal("canceled job remained active")
	}
	listener, err = net.Listen("tcp", p.Config.ListenAddress)
	if err != nil {
		t.Fatalf("challenge listener survived shutdown: %v", err)
	}
	_ = listener.Close()
	if _, err := a.Prepare(p); err == nil {
		t.Fatal("work accepted after shutdown")
	}
}

func TestFailedPreparationReleasesListenerBeforeReportingFailure(t *testing.T) {
	a := NewAutomation(t.TempDir())
	defer a.Close()
	p := managedPolicy(t, AutomaticPublic)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p.Config.ListenAddress = listener.Addr().String()
	_ = listener.Close()
	a.issue = func(context.Context, *Policy, *challengeSolver) ([]byte, error) {
		return nil, errors.New("validation failed")
	}
	if _, err := a.Prepare(p); err != nil {
		t.Fatal(err)
	}
	if status := awaitPreparation(t, a); status.State != "failed" {
		t.Fatalf("expected preparation failure: %+v", status)
	}
	listener, err = net.Listen("tcp", p.Config.ListenAddress)
	if err != nil {
		t.Fatalf("preparation reported failure before releasing the validation listener: %v", err)
	}
	_ = listener.Close()
}

func TestShortCertificateRenewalUsesActualLifetime(t *testing.T) {
	now := time.Now()
	status := CertificateStatus{NotBefore: now, NotAfter: now.Add(160 * time.Hour)}
	if managedRenewalDue(status, now.Add(79*time.Hour)) || !managedRenewalDue(status, now.Add(80*time.Hour)) {
		t.Fatal("160-hour certificates need renewal at half lifetime")
	}
	if !managedRenewalDue(status, status.NotAfter.Add(time.Hour)) {
		t.Fatal("expired certificate was not due for renewal")
	}
}
