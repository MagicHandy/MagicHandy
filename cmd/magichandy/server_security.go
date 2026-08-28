package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/httpapi"
)

type serverSecurityFlags struct {
	certificate *string
	privateKey  *string
	requireAuth *bool
}

type serverSecurity struct {
	TLSConfig              *tls.Config
	AuthenticationRequired bool
	AllowedBrowserHosts    []string
	BaseURL                string
}

func addServerSecurityFlags(flags *flag.FlagSet) serverSecurityFlags {
	return serverSecurityFlags{
		certificate: flags.String("tls-cert", "", "PEM certificate chain for HTTPS (requires -tls-key)"),
		privateKey:  flags.String("tls-key", "", "PEM private key for HTTPS (requires -tls-cert)"),
		requireAuth: flags.Bool("require-auth", false, "require a user account for all non-emergency routes, including on loopback"),
	}
}

func prepareServerRuntime(
	store *config.Store,
	settings config.Settings,
	addressOverride string,
	flags serverSecurityFlags,
	runtime httpapi.Runtime,
) (httpapi.Runtime, serverSecurity, string, error) {
	accountStore, err := accounts.New(store.Datastore())
	if err != nil {
		return runtime, serverSecurity{}, "", err
	}
	enabledAccounts, err := accountStore.EnabledCount(context.Background())
	if err != nil {
		return runtime, serverSecurity{}, "", err
	}
	address := listenAddress(config.Default().Server.Address, settings.Server.Port, addressOverride)
	security, err := resolveServerSecurity(address, *flags.certificate, *flags.privateKey, *flags.requireAuth, enabledAccounts)
	if err != nil {
		return runtime, serverSecurity{}, "", err
	}
	runtime.Accounts = accountStore
	runtime.AuthenticationRequired = security.AuthenticationRequired
	runtime.SecureCookies = security.TLSConfig != nil
	runtime.AllowedBrowserHosts = security.AllowedBrowserHosts
	return runtime, security, address, nil
}

func resolveServerSecurity(address, certificatePath, privateKeyPath string, requireAuth bool, enabledAccounts int) (serverSecurity, error) {
	host, loopback, err := validateListenHost(address)
	if err != nil {
		return serverSecurity{}, err
	}
	certificatePath = strings.TrimSpace(certificatePath)
	privateKeyPath = strings.TrimSpace(privateKeyPath)
	if (certificatePath == "") != (privateKeyPath == "") {
		return serverSecurity{}, errors.New("tls-cert and tls-key must be provided together")
	}
	tlsEnabled := certificatePath != ""
	if !loopback && !tlsEnabled {
		return serverSecurity{}, fmt.Errorf("HTTPS is required for non-loopback listen address %q", address)
	}
	if !loopback && enabledAccounts == 0 {
		return serverSecurity{}, fmt.Errorf("at least one enabled user account is required for non-loopback listen address %q", address)
	}
	result := serverSecurity{
		// Creating the first account is the durable local opt-in. An existing
		// account must never become an inert credential merely because the next
		// launch omitted -require-auth.
		AuthenticationRequired: requireAuth || !loopback || enabledAccounts > 0,
		BaseURL:                "http://" + address,
	}
	if loopback {
		result.AllowedBrowserHosts = nil
	} else {
		result.AllowedBrowserHosts = []string{host}
	}
	if !tlsEnabled {
		return result, nil
	}

	pair, err := tls.LoadX509KeyPair(certificatePath, privateKeyPath)
	if err != nil {
		return serverSecurity{}, fmt.Errorf("load HTTPS certificate and key: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return serverSecurity{}, errors.New("HTTPS certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return serverSecurity{}, fmt.Errorf("parse HTTPS leaf certificate: %w", err)
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return serverSecurity{}, fmt.Errorf("HTTPS certificate is not valid before %s", leaf.NotBefore.Format(time.RFC3339))
	}
	if !now.Before(leaf.NotAfter) {
		return serverSecurity{}, fmt.Errorf("HTTPS certificate expired at %s", leaf.NotAfter.Format(time.RFC3339))
	}
	if err := leaf.VerifyHostname(host); err != nil {
		return serverSecurity{}, fmt.Errorf("HTTPS certificate does not cover listen host %q: %w", host, err)
	}
	pair.Leaf = leaf
	result.TLSConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{pair},
	}
	result.BaseURL = "https://" + address
	if loopback {
		result.AllowedBrowserHosts = []string{host}
	}
	return result, nil
}

func validateListenAddress(address string, tlsEnabled, authenticationReady bool) error {
	_, loopback, err := validateListenHost(address)
	if err != nil {
		return err
	}
	if loopback {
		return nil
	}
	if !tlsEnabled {
		return fmt.Errorf("HTTPS is required for non-loopback listen address %q", address)
	}
	if !authenticationReady {
		return fmt.Errorf("an enabled user account is required for non-loopback listen address %q", address)
	}
	return nil
}

func validateListenHost(address string) (host string, loopback bool, err error) {
	host, _, err = net.SplitHostPort(address)
	if err != nil {
		return "", false, fmt.Errorf("invalid HTTP listen address %q: %w", address, err)
	}
	if strings.EqualFold(host, "localhost") {
		return "localhost", true, nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", false, fmt.Errorf("listen address %q must use localhost or an explicit IP address", address)
	}
	if ip.IsUnspecified() {
		return "", false, fmt.Errorf("listen address %q is a wildcard; bind one explicit interface address so certificate and origin checks stay exact", address)
	}
	loopback = ip.IsLoopback()
	if !loopback && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
		return "", false, fmt.Errorf("listen address %q is not loopback, private, or link-local; internet-facing control is unsupported", address)
	}
	return ip.String(), loopback, nil
}

func newHTTPServer(address string, handler http.Handler, tlsConfig *tls.Config) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
}

func serveHTTP(server *http.Server) error {
	if server.TLSConfig != nil {
		// Certificates are loaded and validated before the API starts; empty paths
		// tell net/http to use TLSConfig.Certificates without reading them again.
		return server.ListenAndServeTLS("", "")
	}
	return server.ListenAndServe()
}
