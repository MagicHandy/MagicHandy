package netaccess

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"
)

func cachedCertificateWithDates(t *testing.T, a *Automation, policy *Policy, before, after time.Time) {
	t.Helper()
	if err := preparePrivateDirectory(a.root); err != nil {
		t.Fatal(err)
	}
	data, err := issueLocalCertificate(a.root, policy.Origin.Hostname())
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(data, data)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	ca, err := localAuthority(a.root)
	if err != nil {
		t.Fatal(err)
	}
	leaf.NotBefore, leaf.NotAfter = before, after
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca.Leaf, leaf.PublicKey, ca.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	data = append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})...)
	if err := writePrivate(certificatePath(a.root, policy), data); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticRenewalRecoversExpiredCacheWithoutServingIt(t *testing.T) {
	a := NewAutomation(t.TempDir())
	defer a.Close()
	p := managedPolicy(t, LocalCA)
	now := time.Now()
	cachedCertificateWithDates(t, a, p, now.Add(-3*time.Minute), now.Add(-time.Minute))
	provider, err := a.LoadManaged(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.TLSConfig().GetCertificate(&tls.ClientHelloInfo{}); err == nil {
		t.Fatal("served expired certificate")
	}
	a.StartRenewal()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !provider.Status().NotAfter.After(now) {
		time.Sleep(10 * time.Millisecond)
	}
	if provider.Status().NotAfter.Before(now.Add(24 * time.Hour)) {
		t.Fatal("expired identity did not renew")
	}
	if _, err := provider.TLSConfig().GetCertificate(&tls.ClientHelloInfo{}); err != nil {
		t.Fatal(err)
	}
	if provider.Status().ReloadError {
		t.Fatal("successful renewal still reports failure")
	}
}

func TestFailedRenewalRetainsCertificateAndFailureStatus(t *testing.T) {
	a := NewAutomation(t.TempDir())
	defer a.Close()
	p := managedPolicy(t, AutomaticPublic)
	now := time.Now()
	cachedCertificateWithDates(t, a, p, now.Add(-3*time.Minute), now.Add(time.Minute))
	provider, err := a.LoadManaged(p)
	if err != nil {
		t.Fatal(err)
	}
	original := provider.Status().Fingerprint
	a.issue = func(context.Context, *Policy, *challengeSolver) ([]byte, error) {
		return nil, errors.New("certificate authority is unavailable")
	}
	a.StartRenewal()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !a.active.renewalFailed.Load() {
		time.Sleep(10 * time.Millisecond)
	}
	if !provider.Status().ReloadError {
		t.Fatal("renewal failure not reported")
	}
	// A file reload succeeding must not hide a failed attempt to renew it.
	a.active.mu.Lock()
	a.active.checked = now.Add(-time.Minute)
	a.active.mu.Unlock()
	if _, err := provider.TLSConfig().GetCertificate(&tls.ClientHelloInfo{}); err != nil {
		t.Fatal(err)
	}
	if provider.Status().Fingerprint != original || !provider.Status().ReloadError {
		t.Fatal("lost previous valid chain or hid renewal failure")
	}
}
