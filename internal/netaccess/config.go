// Package netaccess validates the installation's explicit network boundary.
// It has no authority to start motion, launch processes, or change a firewall.
package netaccess

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	// Local keeps remote exposure disabled.
	Local = "local"
	// DirectHTTPS terminates client TLS in MagicHandy.
	DirectHTTPS = "direct_https"
	// TrustedProxy accepts HTTPS metadata only from explicitly configured peers.
	TrustedProxy = "trusted_proxy"
)

// Config is persisted separately from live settings: changes apply on restart.
// Certificate paths belong to the host. Private-key contents never enter JSON.
type Config struct {
	Mode            string   `json:"mode"`
	ListenAddress   string   `json:"listen_address"`
	PublicURL       string   `json:"public_url"`
	TrustedProxies  []string `json:"trusted_proxies"`
	TLSCertificate  string   `json:"tls_certificate"`
	TLSPrivateKey   string   `json:"tls_private_key"`
	Scope           string   `json:"scope,omitempty"`
	CertificateMode string   `json:"certificate_mode,omitempty"`
	AcceptedTerms   string   `json:"accepted_terms,omitempty"`
}

// Policy is an immutable, validated listener and origin policy.
type Policy struct {
	Config Config
	Origin *url.URL
	Peers  []netip.Prefix
}

// Validate rejects ambiguous origins, wildcard trust and insecure remote modes.
// It does not bind a socket, resolve DNS or read certificate/key files.
func Validate(config Config) (*Policy, error) {
	config.Mode = strings.TrimSpace(config.Mode)
	config.ListenAddress = strings.TrimSpace(config.ListenAddress)
	config.PublicURL = strings.TrimSpace(config.PublicURL)
	ip, address, err := listenAddress(config.ListenAddress)
	if err != nil {
		return nil, err
	}
	config.ListenAddress = address
	policy := &Policy{Config: config}
	if config.Mode == Local {
		if !ip.IsLoopback() || config.PublicURL != "" || len(config.TrustedProxies) != 0 || config.TLSCertificate != "" || config.TLSPrivateKey != "" || config.Scope != "" || config.CertificateMode != "" || config.AcceptedTerms != "" {
			return nil, errors.New("local mode requires a loopback listener without public URL, proxy or TLS settings")
		}
		return policy, nil
	}
	if config.Mode != DirectHTTPS && config.Mode != TrustedProxy {
		return nil, errors.New("network mode must be local, direct_https or trusted_proxy")
	}
	policy.Origin, err = ParseOrigin(config.PublicURL)
	if err != nil {
		return nil, err
	}
	policy.Config.PublicURL = policy.Origin.String()
	if err := validateAutomation(policy, ip); err != nil {
		return nil, err
	}
	if config.Mode == DirectHTTPS {
		if (config.CertificateMode == "" && (strings.TrimSpace(config.TLSCertificate) == "" || strings.TrimSpace(config.TLSPrivateKey) == "")) || len(config.TrustedProxies) != 0 {
			return nil, errors.New("direct HTTPS requires a certificate and private key, without trusted proxy entries")
		}
		return policy, nil
	}
	return validateProxy(policy, ip)
}

func listenAddress(address string) (netip.Addr, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return netip.Addr{}, "", errors.New("listen address must contain an explicit IP and port")
	}
	ip, err := netip.ParseAddr(host)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || ip.Zone() != "" || ip.IsMulticast() || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return netip.Addr{}, "", errors.New("listen address requires an unscoped unicast IP and a port from 1 to 65535")
	}
	return ip, net.JoinHostPort(ip.String(), strconv.Itoa(portNumber)), nil
}

func validateProxy(policy *Policy, listenIP netip.Addr) (*Policy, error) {
	config := policy.Config
	if config.TLSCertificate != "" || config.TLSPrivateKey != "" {
		return nil, errors.New("trusted proxy mode terminates TLS at the proxy; remove backend TLS paths")
	}
	if !listenIP.IsLoopback() && !listenIP.IsPrivate() {
		return nil, errors.New("trusted proxy backend must bind one loopback or private IP")
	}
	if len(config.TrustedProxies) == 0 || len(config.TrustedProxies) > 16 {
		return nil, errors.New("configure between 1 and 16 trusted proxy IPs or CIDRs")
	}
	for _, value := range config.TrustedProxies {
		value = strings.TrimSpace(value)
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			ip, ipErr := netip.ParseAddr(value)
			if ipErr != nil || ip.Zone() != "" {
				return nil, errors.New("trusted proxy entries must be IPs or CIDRs")
			}
			prefix = netip.PrefixFrom(ip, ip.BitLen())
		}
		if prefix.Bits() == 0 || prefix.Addr().Is4In6() || prefix.Addr().IsMulticast() || prefix.Addr().IsUnspecified() {
			return nil, errors.New("trusted proxy entries must identify specific peers; wildcard and mapped ranges are not allowed")
		}
		policy.Peers = append(policy.Peers, prefix.Masked())
	}
	return policy, nil
}

// ParseOrigin accepts one canonical HTTPS origin, with no credentials or path.
func ParseOrigin(value string) (*url.URL, error) {
	origin, err := url.Parse(value)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil ||
		(origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.Opaque != "" {
		return nil, errors.New("public URL must be one HTTPS origin without credentials, a path, query or fragment")
	}
	host, err := originHostname(origin)
	if err != nil {
		return nil, err
	}
	port := origin.Port()
	if port != "" {
		value, portErr := strconv.Atoi(port)
		if portErr != nil || value < 1 || value > 65535 {
			return nil, errors.New("public URL has an invalid port")
		}
		port = strconv.Itoa(value)
	}
	origin.Host = host
	if strings.Contains(host, ":") {
		origin.Host = "[" + host + "]"
	}
	if port != "" && port != "443" {
		origin.Host = net.JoinHostPort(host, port)
	}
	origin.Path, origin.RawPath = "", ""
	return origin, nil
}

func originHostname(origin *url.URL) (string, error) {
	host := strings.ToLower(origin.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.IsUnspecified() || ip.IsMulticast() || ip.Zone() != "" {
			return "", errors.New("public URL cannot advertise a wildcard, scoped or multicast address")
		}
		return ip.String(), nil
	}
	if len(host) == 0 || len(host) > 253 {
		return "", errors.New("public URL has an invalid DNS hostname")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("public URL must use an exact DNS hostname or IP")
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return "", errors.New("public URL must use an ASCII DNS hostname (punycode for international names)")
		}
	}
	return host, nil
}

// Load returns nil for an installation that still uses legacy startup flags.
// A malformed saved policy fails closed instead of falling back to local HTTP.
func Load(ctx context.Context, db *appstore.DB) (*Config, error) {
	var data string
	err := db.SQL().QueryRowContext(ctx, "SELECT value FROM app_kv WHERE key = 'network_config'").Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read network configuration: %w", err)
	}
	if len(data) > 16<<10 {
		return nil, errors.New("saved network configuration exceeds its size limit")
	}
	var config Config
	if err := json.Unmarshal([]byte(data), &config); err != nil {
		return nil, errors.New("saved network configuration is invalid; use -network-mode local for authenticated recovery")
	}
	return &config, nil
}

// Save persists a previously validated configuration atomically in the shared DB.
func Save(ctx context.Context, db *appstore.DB, config Config) error {
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		return SaveTx(ctx, tx, config)
	})
}

// SaveTx validates and persists configuration inside the caller's transaction,
// allowing the HTTP edge to bind the write to current account authority.
func SaveTx(ctx context.Context, tx *sql.Tx, config Config) error {
	policy, err := Validate(config)
	if err != nil {
		return err
	}
	data, err := json.Marshal(policy.Config)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_kv(key, value, updated_at)
			VALUES ('network_config', ?, ?) ON CONFLICT(key) DO UPDATE SET
			value = excluded.value, updated_at = excluded.updated_at`, string(data), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
