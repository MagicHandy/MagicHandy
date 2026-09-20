package main

import (
	"crypto/tls"
	"flag"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

func TestDirectHTTPSSeparatesBindAddressFromCertificateIdentity(t *testing.T) {
	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cert, key := writeServerCertificate(t, "control.example.test", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	securityFlags := addServerSecurityFlags(flags)
	if err := flags.Parse([]string{"-network-mode", "direct_https", "-public-url", "https://control.example.test", "-tls-cert", cert, "-tls-key", key}); err != nil {
		t.Fatal(err)
	}
	security, address, err := resolveConfiguredNetwork(store, "0.0.0.0:49717", "0.0.0.0:49717", securityFlags, 1)
	if err != nil {
		t.Fatal(err)
	}
	if address != "0.0.0.0:49717" || security.BaseURL != "https://control.example.test" || !security.AuthenticationRequired {
		t.Fatalf("wrong direct boundary: %+v %s", security, address)
	}
	pair, err := security.TLSConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "control.example.test"})
	if err != nil || pair.Leaf.VerifyHostname("control.example.test") != nil {
		t.Fatalf("wrong public TLS identity: %v", err)
	}
	if _, _, err := resolveConfiguredNetwork(store, address, address, securityFlags, 0); err == nil {
		t.Fatal("direct exposure without initialized accounts")
	}
}

func TestLocalRecoveryOverridesBrokenRemoteConfigurationAndRetainsAuth(t *testing.T) {
	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	config := netaccess.Config{Mode: netaccess.DirectHTTPS, ListenAddress: "0.0.0.0:49717", PublicURL: "https://control.example.test", TLSCertificate: "missing-cert", TLSPrivateKey: "missing-key"}
	if err := netaccess.Save(t.Context(), store.Datastore(), config); err != nil {
		t.Fatal(err)
	}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	securityFlags := addServerSecurityFlags(flags)
	if _, _, err := resolveConfiguredNetwork(store, "127.0.0.1:49717", "", securityFlags, 1); err == nil {
		t.Fatal("broken TLS fell back silently")
	}
	if err := flags.Parse([]string{"-network-mode", "local"}); err != nil {
		t.Fatal(err)
	}
	security, address, err := resolveConfiguredNetwork(store, "127.0.0.1:49717", "", securityFlags, 1)
	if err != nil || !security.AuthenticationRequired || address != "127.0.0.1:49717" || security.TLSConfig != nil {
		t.Fatalf("recovery lost local protection: %+v %s %v", security, address, err)
	}
	saved, err := netaccess.Load(t.Context(), store.Datastore())
	if err != nil || saved.Mode != netaccess.DirectHTTPS {
		t.Fatal("recovery destroyed the saved remote configuration")
	}
}
