package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveServerSecurityKeepsLoopbackHTTPDefault(t *testing.T) {
	security, err := resolveServerSecurity("127.0.0.1:49717", "", "", false, 0)
	if err != nil {
		t.Fatalf("resolveServerSecurity: %v", err)
	}
	if security.TLSConfig != nil || security.AuthenticationRequired || security.BaseURL != "http://127.0.0.1:49717" {
		t.Fatalf("loopback security = %+v", security)
	}
}

func TestResolveServerSecurityRequiresAccountAndMatchingCertificateForLAN(t *testing.T) {
	certificatePath, keyPath := writeServerCertificate(t, "192.168.1.20", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if _, err := resolveServerSecurity("192.168.1.20:49717", "", "", false, 1); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("LAN without TLS error = %v", err)
	}
	if _, err := resolveServerSecurity("192.168.1.20:49717", certificatePath, keyPath, false, 0); err == nil || !strings.Contains(err.Error(), "account") {
		t.Fatalf("LAN without account error = %v", err)
	}
	security, err := resolveServerSecurity("192.168.1.20:49717", certificatePath, keyPath, false, 1)
	if err != nil {
		t.Fatalf("resolve LAN security: %v", err)
	}
	if security.TLSConfig == nil || security.TLSConfig.MinVersion != tls.VersionTLS12 ||
		!security.AuthenticationRequired || security.BaseURL != "https://192.168.1.20:49717" ||
		len(security.AllowedBrowserHosts) != 1 || security.AllowedBrowserHosts[0] != "192.168.1.20" {
		t.Fatalf("LAN security = %+v", security)
	}

	mismatchCertificate, mismatchKey := writeServerCertificate(t, "192.168.1.21", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if _, err := resolveServerSecurity("192.168.1.20:49717", mismatchCertificate, mismatchKey, false, 1); err == nil || !strings.Contains(err.Error(), "does not cover") {
		t.Fatalf("mismatched certificate error = %v", err)
	}
}

func TestResolveServerSecurityRejectsIncompleteExpiredAndWildcardTLS(t *testing.T) {
	if _, err := resolveServerSecurity("127.0.0.1:49717", "cert.pem", "", false, 0); err == nil || !strings.Contains(err.Error(), "together") {
		t.Fatalf("partial TLS error = %v", err)
	}
	expiredCertificate, expiredKey := writeServerCertificate(t, "127.0.0.1", time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	if _, err := resolveServerSecurity("127.0.0.1:49717", expiredCertificate, expiredKey, false, 0); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired certificate error = %v", err)
	}
	if _, err := resolveServerSecurity("0.0.0.0:49717", expiredCertificate, expiredKey, false, 1); err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("wildcard binding error = %v", err)
	}
}

func TestResolvedTLSConfigurationServesTrustedHTTPS(t *testing.T) {
	certificatePath, keyPath := writeServerCertificate(t, "127.0.0.1", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	security, err := resolveServerSecurity("127.0.0.1:49717", certificatePath, keyPath, false, 0)
	if err != nil {
		t.Fatalf("resolve TLS security: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := newHTTPServer(listener.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), security.TLSConfig)
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
		<-done
	})

	certificatePEM, err := os.ReadFile(certificatePath) // #nosec G304 -- path comes from this test's private TempDir.
	if err != nil {
		t.Fatalf("read certificate: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificatePEM) {
		t.Fatal("append test certificate")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: "127.0.0.1",
	}}}
	response, err := client.Get("https://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("HTTPS request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNoContent || response.TLS == nil || response.TLS.Version < tls.VersionTLS12 {
		t.Fatalf("HTTPS response = status %d TLS %+v", response.StatusCode, response.TLS)
	}
}

func writeServerCertificate(t *testing.T, host string, notBefore, notAfter time.Time) (string, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 120)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "MagicHandy test"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	var certificatePEM, privatePEM bytes.Buffer
	if err := pem.Encode(&certificatePEM, &pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}); err != nil {
		t.Fatalf("encode certificate: %v", err)
	}
	if err := pem.Encode(&privatePEM, &pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}); err != nil {
		t.Fatalf("encode private key: %v", err)
	}
	directory := t.TempDir()
	certificatePath := filepath.Join(directory, "server.crt")
	privatePath := filepath.Join(directory, "server.key")
	if err := os.WriteFile(certificatePath, certificatePEM.Bytes(), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(privatePath, privatePEM.Bytes(), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	return certificatePath, privatePath
}
