package main

import (
	"errors"
	"flag"
	"net/http"
	"net/url"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/httpapi"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

type remoteFlags struct {
	set       *flag.FlagSet
	port      *int
	publicURL *string
}

func addRemoteFlags(flags *flag.FlagSet) remoteFlags {
	return remoteFlags{
		set:       flags,
		port:      flags.Int("remote-port", 0, "remote interface port (default: app port + 1; -1 disables the remote)"),
		publicURL: flags.String("remote-public-url", "", "remote HTTPS origin, for an external port or trusted reverse proxy"),
	}
}

func prepareRemoteServer(address string, security serverSecurity, flags remoteFlags) (serverSecurity, string, error) {
	port, publicURL := *flags.port, strings.TrimSpace(*flags.publicURL)
	portSet, urlSet := false, false
	flags.set.Visit(func(value *flag.Flag) {
		portSet = portSet || value.Name == "remote-port"
		urlSet = urlSet || value.Name == "remote-public-url"
	})
	if policy := security.NetworkPolicy; policy != nil {
		if !portSet {
			port = policy.Config.RemotePort
		}
		if !urlSet && port != -1 {
			publicURL = policy.Config.RemotePublicURL
		}
	}
	remoteAddress, remoteURL, err := netaccess.RemoteEndpoint(address, security.BaseURL, port, publicURL)
	if err != nil || remoteAddress == "" {
		return serverSecurity{}, "", err
	}
	remote := security
	remote.BaseURL = remoteURL
	remote.NetworkPolicy, err = netaccess.RemotePolicy(security.NetworkPolicy, remoteAddress, remoteURL)
	if err != nil {
		return serverSecurity{}, "", err
	}
	if remote.NetworkPolicy == nil && security.TLSConfig != nil {
		mainOrigin, _ := url.Parse(security.BaseURL)
		remoteOrigin, _ := url.Parse(remoteURL)
		if !strings.EqualFold(mainOrigin.Hostname(), remoteOrigin.Hostname()) {
			return serverSecurity{}, "", errors.New("direct HTTPS remote must use the main app certificate's hostname")
		}
	}
	if remote.NetworkPolicy != nil && remote.NetworkPolicy.Origin != nil {
		remote.AllowedBrowserHosts = []string{remote.NetworkPolicy.Origin.Host}
	}
	return remote, remoteAddress, nil
}

func makeRemoteHTTPServer(api *httpapi.Server, address string, security serverSecurity) *http.Server {
	if address == "" {
		return nil
	}
	return newHTTPServer(address, api.RemoteHandler(httpapi.RemoteInterfaceOptions{
		AllowedBrowserHosts: security.AllowedBrowserHosts, NetworkPolicy: security.NetworkPolicy,
		SecureCookies: security.TLSConfig != nil || (security.NetworkPolicy != nil && security.NetworkPolicy.Config.Mode == netaccess.TrustedProxy),
	}), security.TLSConfig)
}
