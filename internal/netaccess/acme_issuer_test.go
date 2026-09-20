package netaccess

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mholt/acmez/v3"
	"github.com/mholt/acmez/v3/acme"
)

// This CA speaks ACME over a local socket, signs the real CSR, and validates
// the real TLS-ALPN handshake. Tests never register accounts with a public CA.
type certificateCAFixture struct {
	mu        sync.Mutex
	server    *httptest.Server
	root      *tls.Certificate
	policy    *Policy
	validated bool
	chain     []byte
	accounts  int
}

func newCertificateCA(t *testing.T, policy *Policy) *certificateCAFixture {
	t.Helper()
	root, err := localAuthority(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := &certificateCAFixture{root: root, policy: policy}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *certificateCAFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	base := f.server.URL
	w.Header().Set("Replay-Nonce", "fixture-nonce")
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/directory" {
		_ = json.NewEncoder(w).Encode(map[string]any{"newNonce": base + "/nonce", "newAccount": base + "/account", "newOrder": base + "/new-order", "meta": map[string]any{"termsOfService": testTerms, "profiles": map[string]string{"shortlived": "test profile"}}})
		return
	}
	if r.URL.Path == "/nonce" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var envelope struct {
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&envelope); err != nil {
		http.Error(w, "invalid JWS", 400)
		return
	}
	payload, err := base64.RawURLEncoding.DecodeString(envelope.Payload)
	if err != nil {
		http.Error(w, "invalid payload", 400)
		return
	}
	switch r.URL.Path {
	case "/account":
		f.accounts++
		w.Header().Set("Location", base+"/account/1")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"status":"valid"}`)
	case "/new-order":
		if !f.validOrder(payload) {
			http.Error(w, "missing IP certificate profile", http.StatusBadRequest)
			return
		}
		w.Header().Set("Location", base+"/order")
		w.WriteHeader(http.StatusCreated)
		f.order(w)
	case "/authorization":
		status := "pending"
		if f.validated {
			status = "valid"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "identifier": map[string]string{"type": "ip", "value": f.policy.Origin.Hostname()}, "challenges": []map[string]string{{"type": "tls-alpn-01", "url": base + "/challenge", "token": "test-token", "status": status}}})
	case "/challenge":
		if err := f.checkChallenge(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		f.validated = true
		_, _ = fmt.Fprint(w, `{"status":"valid","type":"tls-alpn-01","url":"`+base+`/challenge"}`)
	case "/order":
		f.order(w)
	case "/finalize":
		if !f.validated {
			http.Error(w, "challenge not validated", 400)
			return
		}
		if err := f.sign(payload); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		f.order(w)
	case "/certificate":
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		_, _ = w.Write(f.chain)
	default:
		http.NotFound(w, r)
	}
}

func (f *certificateCAFixture) validOrder(payload []byte) bool {
	var order acme.Order
	return json.Unmarshal(payload, &order) == nil && order.Profile == "shortlived" && len(order.Identifiers) == 1 && order.Identifiers[0].Type == "ip" && order.Identifiers[0].Value == f.policy.Origin.Hostname()
}

func (f *certificateCAFixture) order(w http.ResponseWriter) {
	status := "pending"
	if f.validated {
		status = "ready"
	}
	if len(f.chain) > 0 {
		status = "valid"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "identifiers": []map[string]string{{"type": "ip", "value": f.policy.Origin.Hostname()}}, "authorizations": []string{f.server.URL + "/authorization"}, "finalize": f.server.URL + "/finalize", "certificate": f.server.URL + "/certificate"})
}

func (f *certificateCAFixture) checkChallenge() error {
	// Self-signed ACME challenge certificates are expected. Inspect their
	// critical validation extension and IP SAN instead of normal browser trust.
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", f.policy.Config.ListenAddress,
		&tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, ServerName: challengeServerName(f.policy.Origin.Hostname()), NextProtos: []string{acmez.ACMETLS1Protocol}}) // #nosec G402 -- dedicated local ACME validation fixture.
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	state := connection.ConnectionState()
	if state.NegotiatedProtocol != acmez.ACMETLS1Protocol || len(state.PeerCertificates) != 1 {
		return fmt.Errorf("wrong validation handshake")
	}
	leaf := state.PeerCertificates[0]
	if err := leaf.VerifyHostname(f.policy.Origin.Hostname()); err != nil {
		return err
	}
	for _, extension := range leaf.Extensions {
		if extension.Critical && extension.Id.String() == "1.3.6.1.5.5.7.1.31" {
			return nil
		}
	}
	return fmt.Errorf("missing critical ACME validation extension")
}

func (f *certificateCAFixture) sign(payload []byte) error {
	var final struct {
		CSR string `json:"csr"`
	}
	if err := json.Unmarshal(payload, &final); err != nil {
		return err
	}
	der, err := base64.RawURLEncoding.DecodeString(final.CSR)
	if err != nil {
		return err
	}
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil {
		return err
	}
	if err := csr.CheckSignature(); err != nil {
		return err
	}
	serial, err := serialNumber()
	if err != nil {
		return err
	}
	leaf := &x509.Certificate{SerialNumber: serial, IPAddresses: csr.IPAddresses, DNSNames: csr.DNSNames, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(160 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err = x509.CreateCertificate(rand.Reader, leaf, f.root.Leaf, csr.PublicKey, f.root.PrivateKey)
	if err != nil {
		return err
	}
	f.chain = append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.root.Leaf.Raw})...)
	return nil
}

func TestPublicIPCertificateEndToEnd(t *testing.T) {
	a := NewAutomation(t.TempDir())
	defer a.Close()
	p := managedPolicy(t, AutomaticPublic)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p.Config.ListenAddress = listener.Addr().String()
	_ = listener.Close()
	ca := newCertificateCA(t, p)
	a.issue = func(ctx context.Context, policy *Policy, solver *challengeSolver) ([]byte, error) {
		return a.obtainPublic(ctx, policy, solver, &acme.Client{Directory: ca.server.URL + "/directory", HTTPClient: ca.server.Client(), PollInterval: time.Millisecond})
	}
	if _, err := a.Prepare(p); err != nil {
		t.Fatal(err)
	}
	if status := awaitPreparation(t, a); status.State != "ready" {
		t.Fatalf("public preparation failed: %+v", status)
	}
	if err := a.ValidatePrepared(p); err != nil {
		t.Fatal(err)
	}
	ca.mu.Lock()
	validated, registrations := ca.validated, ca.accounts
	ca.mu.Unlock()
	if !validated || registrations != 1 {
		t.Fatal("did not complete one real certificate validation")
	}
	listener, err = net.Listen("tcp", p.Config.ListenAddress)
	if err != nil {
		t.Fatalf("preparation reported ready before releasing the validation listener: %v", err)
	}
	_ = listener.Close()
	provider, err := a.LoadManaged(p)
	if err != nil {
		t.Fatal(err)
	}
	if provider.Status().RenewalDue || time.Until(provider.Status().NotAfter) < 159*time.Hour {
		t.Fatal("incorrect short-lived certificate lifetime")
	}
}

func TestChangedCATermsDoNotCreateAccount(t *testing.T) {
	a := NewAutomation(t.TempDir())
	defer a.Close()
	p := managedPolicy(t, AutomaticPublic)
	ca := newCertificateCA(t, p)
	p.Config.AcceptedTerms = "https://letsencrypt.org/documents/old-terms.pdf"
	_, err := a.obtainPublic(t.Context(), p, &challengeSolver{host: p.Origin.Hostname()}, &acme.Client{Directory: ca.server.URL + "/directory", HTTPClient: ca.server.Client()})
	if err == nil {
		t.Fatal("accepted changed terms")
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if ca.accounts != 0 {
		t.Fatal("created CA account before consent")
	}
}
