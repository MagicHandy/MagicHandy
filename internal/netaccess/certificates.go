package netaccess

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// CertificateStatus contains public certificate metadata, never file paths/key data.
type CertificateStatus struct {
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	Fingerprint string    `json:"sha256"`
	DNSNames    []string  `json:"dns_names"`
	RenewalDue  bool      `json:"renewal_due"`
	ReloadError bool      `json:"reload_error"`
}

// Certificates retains the last valid chain during a failed file replacement.
// An expired chain is never offered. Replacements are checked at most every 30s
// on a handshake; the operator's CA/ACME client owns issuance and file writes.
type Certificates struct {
	mu       sync.Mutex
	config   Config
	host     string
	pair     *tls.Certificate
	checked  time.Time
	reloadOK bool
}

// LoadCertificates validates the configured identity before a listener starts.
func LoadCertificates(policy *Policy) (*Certificates, error) {
	if policy.Config.Mode != DirectHTTPS || policy.Origin == nil {
		return nil, errors.New("certificate loading requires a direct HTTPS policy")
	}
	certs := &Certificates{config: policy.Config, host: policy.Origin.Hostname()}
	pair, err := loadCertificate(certs.config, certs.host, time.Now())
	if err != nil {
		return nil, err
	}
	certs.pair, certs.checked, certs.reloadOK = pair, time.Now(), true
	return certs, nil
}

func loadCertificate(config Config, host string, now time.Time) (*tls.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(config.TLSCertificate, config.TLSPrivateKey)
	if err != nil {
		// File errors can contain private host paths; the API reports this safe
		// actionable category, without forwarding the filesystem error.
		return nil, errors.New("cannot load the HTTPS certificate and matching private key; check the paths, PEM format and service-account permissions")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, errors.New("cannot parse the HTTPS leaf certificate")
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, errors.New("HTTPS certificate is expired or not yet valid")
	}
	if err := leaf.VerifyHostname(host); err != nil {
		return nil, errors.New("HTTPS certificate does not cover the advertised public hostname or IP")
	}
	if !allowsServerAuth(leaf) {
		return nil, errors.New("HTTPS certificate does not permit server authentication")
	}
	child := leaf
	for _, encoded := range pair.Certificate[1:] {
		parent, err := x509.ParseCertificate(encoded)
		if err != nil || child.CheckSignatureFrom(parent) != nil || now.Before(parent.NotBefore) || !now.Before(parent.NotAfter) {
			return nil, errors.New("HTTPS intermediate certificates must be valid and ordered after the leaf")
		}
		child = parent
	}
	pair.Leaf = leaf
	return &pair, nil
}

func allowsServerAuth(leaf *x509.Certificate) bool {
	if len(leaf.ExtKeyUsage) == 0 && len(leaf.UnknownExtKeyUsage) == 0 {
		return true
	}
	for _, usage := range leaf.ExtKeyUsage {
		if usage == x509.ExtKeyUsageAny || usage == x509.ExtKeyUsageServerAuth {
			return true
		}
	}
	return false
}

// TLSConfig returns a TLS 1.2+ configuration with bounded renewal checks.
func (c *Certificates) TLSConfig() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: c.getCertificate}
}

func (c *Certificates) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if now.Sub(c.checked) >= 30*time.Second {
		pair, err := loadCertificate(c.config, c.host, now)
		c.checked, c.reloadOK = now, err == nil
		if err == nil {
			c.pair = pair
		}
	}
	if !now.Before(c.pair.Leaf.NotAfter) {
		return nil, errors.New("HTTPS certificate expired; replace the configured certificate and key")
	}
	if hello.ServerName != "" && c.pair.Leaf.VerifyHostname(hello.ServerName) != nil {
		return nil, fmt.Errorf("unrecognized HTTPS server name")
	}
	return c.pair, nil
}

// Status snapshots certificate metadata without touching key files.
func (c *Certificates) Status() CertificateStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	leaf := c.pair.Leaf
	digest := sha256.Sum256(leaf.Raw)
	return CertificateStatus{NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter,
		Fingerprint: hex.EncodeToString(digest[:]), DNSNames: append([]string{}, leaf.DNSNames...),
		RenewalDue: time.Until(leaf.NotAfter) < 30*24*time.Hour, ReloadError: !c.reloadOK}
}
