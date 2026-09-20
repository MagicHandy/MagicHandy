package netaccess

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/mholt/acmez/v3"
	"github.com/mholt/acmez/v3/acme"
)

func (a *Automation) issuePublic(ctx context.Context, policy *Policy, solver *challengeSolver) ([]byte, error) {
	client := certificateHTTPClient()
	defer client.CloseIdleConnections()
	ca := &acme.Client{Directory: letsEncryptDirectory, HTTPClient: client, UserAgent: "MagicHandy", PollTimeout: 3 * time.Minute}
	return a.obtainPublic(ctx, policy, solver, ca)
}

func (a *Automation) obtainPublic(ctx context.Context, policy *Policy, solver *challengeSolver, ca *acme.Client) ([]byte, error) {
	directory, err := ca.GetDirectory(ctx)
	if err != nil {
		return nil, errors.New("the certificate authority is unavailable; check Internet connectivity and try again")
	}
	if directory.Meta.TermsOfService != policy.Config.AcceptedTerms {
		return nil, errors.New("the certificate authority terms changed; review and accept the current terms in HTTPS setup")
	}
	if _, ok := directory.Meta.Profiles["shortlived"]; !ok {
		return nil, errors.New("the certificate authority does not offer the required short-lived certificate profile")
	}
	accountKey, err := loadOrCreateAccountKey(a.root)
	if err != nil {
		return nil, err
	}
	// The persisted exact terms URL is explicit administrator consent. Reusing
	// the durable key returns the existing ACME account after process restarts.
	account, err := ca.NewAccount(ctx, acme.Account{PrivateKey: accountKey, TermsOfServiceAgreed: true})
	if err != nil {
		return nil, publicCertificateError(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, errors.New("could not generate a certificate key")
	}
	csr, err := acmez.NewCSR(key, []string{policy.Origin.Hostname()})
	if err != nil {
		return nil, errors.New("could not create a certificate request")
	}
	order, err := acmez.OrderParametersFromCSR(account, csr)
	if err != nil {
		return nil, errors.New("could not create a certificate order")
	}
	order.Profile = "shortlived"
	// ARI replacement information avoids counting successful renewals as new
	// identifiers on authorities that support it. Scheduling remains conservative
	// at half the actual certificate lifetime, including 160-hour IP certificates.
	if previous, err := a.load(policy); err == nil {
		order.Replaces = previous.pair.Leaf
	}
	issuer := &acmez.Client{Client: ca, ChallengeSolvers: map[string]acmez.Solver{acme.ChallengeTypeTLSALPN01: solver}}
	chains, err := issuer.ObtainCertificate(ctx, order)
	if err != nil {
		return nil, publicCertificateError(err)
	}
	if len(chains) == 0 {
		return nil, errors.New("the certificate authority returned no certificate")
	}
	keyPEM, err := encodeKey(key)
	if err != nil {
		return nil, errors.New("could not store the new certificate key")
	}
	return append(chains[0].ChainPEM, keyPEM...), nil
}

func publicCertificateError(err error) error {
	var problem acme.Problem
	if errors.As(err, &problem) && strings.HasSuffix(problem.Type, ":rateLimited") {
		return errors.New("the certificate authority rate limit was reached; wait before retrying and verify DNS, port forwarding, and the firewall first")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("certificate preparation was canceled")
	}
	return errors.New("public certificate validation failed; Internet TCP port 443 must reach the selected local port. Check the public IP or DNS, router port forwarding, firewall exclusion, and ISP restrictions before retrying")
}

type challengeSolver struct {
	mu   sync.RWMutex
	host string
	pair *tls.Certificate
}

func (s *challengeSolver) Present(_ context.Context, challenge acme.Challenge) error {
	if challenge.Identifier.Value != s.host {
		return errors.New("unexpected certificate challenge identifier")
	}
	pair, err := acmez.TLSALPN01ChallengeCert(challenge)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.pair = pair
	s.mu.Unlock()
	return nil
}

func (s *challengeSolver) CleanUp(context.Context, acme.Challenge) error {
	s.mu.Lock()
	s.pair = nil
	s.mu.Unlock()
	return nil
}

func (s *challengeSolver) config(normal func(*tls.ClientHelloInfo) (*tls.Certificate, error)) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: normal,
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			if len(hello.SupportedProtos) != 1 || hello.SupportedProtos[0] != acmez.ACMETLS1Protocol {
				if normal == nil {
					return nil, errors.New("certificate setup serves only validation handshakes")
				}
				return nil, nil
			}
			s.mu.RLock()
			pair := s.pair
			s.mu.RUnlock()
			if pair == nil || !strings.EqualFold(strings.TrimSuffix(hello.ServerName, "."), challengeServerName(s.host)) {
				return nil, errors.New("no active validation challenge for this server name")
			}
			return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{*pair}, NextProtos: []string{acmez.ACMETLS1Protocol}}, nil
		}}
}

// RFC 8738 validation uses reverse-address SNI, unlike ordinary IP HTTPS (which
// normally omits SNI). Accept it only on the dedicated ACME ALPN handshake.
func challengeServerName(host string) string {
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	if ip.Is4() {
		octets := ip.As4()
		return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", octets[3], octets[2], octets[1], octets[0])
	}
	bytes := ip.As16()
	const hex = "0123456789abcdef"
	var name strings.Builder
	for i := len(bytes) - 1; i >= 0; i-- {
		name.WriteByte(hex[bytes[i]&15])
		name.WriteByte('.')
		name.WriteByte(hex[bytes[i]>>4])
		name.WriteByte('.')
	}
	name.WriteString("ip6.arpa")
	return name.String()
}
