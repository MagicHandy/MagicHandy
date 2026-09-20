package main

import (
	"context"
	"errors"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

func flagString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func resolveConfiguredNetwork(store *config.Store, defaultAddress, addressOverride string, flags serverSecurityFlags, enabledAccounts int) (serverSecurity, string, error) {
	network, err := networkConfiguration(store, defaultAddress, addressOverride, flags)
	if err != nil {
		return serverSecurity{}, "", err
	}
	if network == nil {
		security, err := resolveServerSecurity(defaultAddress, flagString(flags.certificate), flagString(flags.privateKey), *flags.requireAuth, enabledAccounts)
		return security, defaultAddress, err
	}
	policy, err := netaccess.Validate(*network)
	if err != nil {
		return serverSecurity{}, "", err
	}
	security := serverSecurity{NetworkPolicy: policy, AuthenticationRequired: *flags.requireAuth || enabledAccounts > 0,
		BaseURL: "http://" + policy.Config.ListenAddress}
	if policy.Config.Mode == netaccess.Local {
		return security, policy.Config.ListenAddress, nil
	}
	if enabledAccounts == 0 {
		return serverSecurity{}, "", errors.New("remote access requires an enabled account; create the administrator using -network-mode local first")
	}
	security.AuthenticationRequired = true
	security.BaseURL = policy.Config.PublicURL
	security.AllowedBrowserHosts = []string{policy.Origin.Host}
	if policy.Config.Mode == netaccess.DirectHTTPS {
		if policy.Config.CertificateMode != "" {
			security.Automation = netaccess.NewAutomation(store.DataDir())
			security.Certificates, err = security.Automation.LoadManaged(policy)
		} else {
			security.Certificates, err = netaccess.LoadCertificates(policy)
		}
		if err != nil {
			if security.Automation != nil {
				security.Automation.Close()
			}
			return serverSecurity{}, "", err
		}
		security.TLSConfig = security.Certificates.TLSConfig()
	}
	return security, policy.Config.ListenAddress, nil
}

func networkConfiguration(store *config.Store, defaultAddress, addressOverride string, flags serverSecurityFlags) (*netaccess.Config, error) {
	mode := flagString(flags.networkMode)
	// An explicit local recovery launch must work even when saved remote
	// configuration is malformed. It never deletes accounts or disables login.
	if mode == netaccess.Local {
		return &netaccess.Config{Mode: netaccess.Local, ListenAddress: defaultAddress}, nil
	}
	network, err := netaccess.Load(context.Background(), store.Datastore())
	if err != nil {
		return nil, err
	}
	if network == nil && mode == "" {
		if flagString(flags.publicURL) != "" || flagString(flags.trustedProxies) != "" {
			return nil, errors.New("public-url and trusted-proxies require an explicit network-mode")
		}
		return nil, nil
	}
	if network == nil || mode != "" {
		network = &netaccess.Config{Mode: mode, ListenAddress: defaultAddress}
	}
	if addressOverride != "" {
		network.ListenAddress = addressOverride
	}
	if value := flagString(flags.publicURL); value != "" {
		network.PublicURL = value
	}
	if value := flagString(flags.trustedProxies); value != "" {
		network.TrustedProxies = strings.Split(value, ",")
	}
	if value := flagString(flags.certificate); value != "" {
		network.TLSCertificate = value
	}
	if value := flagString(flags.privateKey); value != "" {
		network.TLSPrivateKey = value
	}
	return network, nil
}
