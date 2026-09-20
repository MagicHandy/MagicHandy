package netaccess

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

const (
	// AutomaticPublic obtains and renews a public certificate using TLS-ALPN-01.
	AutomaticPublic = "automatic_public"
	// LocalCA uses this installation's private CA; clients explicitly enroll trust.
	LocalCA = "local_ca"
)

func validateAutomation(policy *Policy, ip netip.Addr) error {
	c := policy.Config
	if c.Scope != "" && c.Scope != "lan" && c.Scope != "public" {
		return errors.New("access scope must be lan or public")
	}
	if (ip.IsUnspecified() && c.CertificateMode != "") || (c.Scope == "lan" && !ip.IsPrivate() && !ip.IsLoopback()) {
		return errors.New("select one local interface address; LAN access requires a private or loopback address")
	}
	if c.CertificateMode == "" {
		if c.AcceptedTerms != "" {
			return errors.New("certificate terms apply only to automatic public certificates")
		}
		return nil
	}
	if c.Mode != DirectHTTPS || c.TLSCertificate != "" || c.TLSPrivateKey != "" {
		return errors.New("automatic certificates require direct HTTPS without certificate file paths")
	}
	switch c.CertificateMode {
	case LocalCA:
		hostIP, err := netip.ParseAddr(policy.Origin.Hostname())
		if c.Scope != "lan" || c.AcceptedTerms != "" || err != nil || hostIP != ip {
			return errors.New("LAN certificates require the selected local IP as the HTTPS address")
		}
	case AutomaticPublic:
		return validatePublicAutomation(policy)
	default:
		return errors.New("unknown automatic certificate mode")
	}
	return nil
}

func validatePublicAutomation(policy *Policy) error {
	c := policy.Config
	if c.Scope != "public" || policy.Origin.Port() != "" {
		return errors.New("automatic public HTTPS requires public scope and external TCP port 443")
	}
	host := policy.Origin.Hostname()
	if address, err := netip.ParseAddr(host); err == nil {
		if !publicIP(address) {
			return errors.New("public certificates require a globally routable public IP or DNS name")
		}
	} else if !strings.Contains(host, ".") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") {
		return errors.New("public certificates require a public DNS name or IP")
	}
	terms, err := url.Parse(c.AcceptedTerms)
	if err != nil || terms.Scheme != "https" || terms.Host != "letsencrypt.org" || terms.User != nil || terms.RawQuery != "" || terms.Fragment != "" || !strings.HasPrefix(terms.Path, "/documents/") || len(c.AcceptedTerms) > 512 {
		return errors.New("read and accept the current Let's Encrypt terms before requesting a certificate")
	}
	return nil
}

// Exclude private, documentation, shared NAT and special-purpose addresses.
func publicIP(ip netip.Addr) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.Is4In6() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/23", "3fff::/20"} {
		if netip.MustParsePrefix(block).Contains(ip) {
			return false
		}
	}
	return true
}
