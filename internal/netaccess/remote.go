package netaccess

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// RemoteEndpoint derives a second origin without widening the bind interface,
// TLS policy or trusted proxy peers. A remote port of -1 disables the listener;
// zero selects the port after the main listener. Public origin overrides are
// useful for a trusted proxy. Direct HTTPS reuses the main host's certificate.
func RemoteEndpoint(address, baseURL string, port int, publicURL string) (string, string, error) {
	if port == -1 {
		if publicURL != "" {
			return "", "", errors.New("remove the remote public URL when the remote listener is disabled")
		}
		return "", "", nil
	}
	host, mainPort, err := net.SplitHostPort(address)
	if err != nil {
		return "", "", err
	}
	current, err := strconv.Atoi(mainPort)
	if err != nil {
		return "", "", err
	}
	automatic := port == 0
	if automatic {
		port = current + 1
	}
	if port < 1 || port > 65535 || port == current {
		return "", "", errors.New("remote port must be distinct from the app port and between 1 and 65535; use -1 to disable it")
	}
	origin, err := url.Parse(baseURL)
	if err != nil || origin.Hostname() == "" {
		return "", "", errors.New("the main app origin is unavailable")
	}
	baseOrigin := *origin
	// A proxy may already expose the main app on the adjacent external port.
	// Automatic selection skips that origin; explicit collisions are rejected.
	if automatic && publicURL == "" && strconv.Itoa(port) == originPort(&baseOrigin) {
		port++
		if port > 65535 {
			return "", "", errors.New("choose an explicit remote port below 65535")
		}
	}
	origin.Host = net.JoinHostPort(origin.Hostname(), strconv.Itoa(port))
	if publicURL != "" {
		if origin.Scheme != "https" {
			return "", "", errors.New("a remote public URL requires HTTPS network setup")
		}
		origin, err = ParseOrigin(publicURL)
		if err != nil {
			return "", "", err
		}
	}
	if sameOrigin(origin, &baseOrigin) {
		return "", "", errors.New("the app and remote must use distinct public origins")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), origin.String(), nil
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && originPort(a) == originPort(b)
}

func validateRemoteConfig(config Config) error {
	if config.RemotePort < -1 || config.RemotePort > 65535 {
		return errors.New("remote port must be -1, 0, or between 1 and 65535")
	}
	base := config.PublicURL
	if config.Mode == Local {
		base = "http://" + config.ListenAddress
	}
	_, remoteURL, err := RemoteEndpoint(config.ListenAddress, base, config.RemotePort, config.RemotePublicURL)
	if err != nil {
		return err
	}
	if config.Mode == DirectHTTPS && config.RemotePublicURL != "" {
		mainOrigin, err := ParseOrigin(base)
		if err != nil {
			return err
		}
		remoteOrigin, err := ParseOrigin(remoteURL)
		if err != nil || !strings.EqualFold(remoteOrigin.Hostname(), mainOrigin.Hostname()) {
			return errors.New("direct HTTPS remote must use the main app certificate hostname")
		}
	}
	return nil
}

// RemotePolicy reuses a validated policy and its certificate provider. ACME
// issuance remains on the main public 443 listener; serving the resulting
// certificate on the remote port does not request a second certificate.
func RemotePolicy(parent *Policy, address, publicURL string) (*Policy, error) {
	if parent == nil {
		return nil, nil
	}
	child := *parent
	child.Config = parent.Config
	child.Config.ListenAddress = address
	child.Config.RemotePort, child.Config.RemotePublicURL = 0, ""
	if parent.Config.Mode == Local {
		return &child, nil
	}
	origin, err := ParseOrigin(publicURL)
	if err != nil {
		return nil, err
	}
	if parent.Config.Mode == DirectHTTPS && !strings.EqualFold(origin.Hostname(), parent.Origin.Hostname()) {
		return nil, errors.New("direct HTTPS remote must use the main app certificate's hostname")
	}
	child.Origin = origin
	child.Config.PublicURL = origin.String()
	return &child, nil
}

func originPort(origin *url.URL) string {
	if port := origin.Port(); port != "" {
		return port
	}
	if origin.Scheme == "https" {
		return "443"
	}
	return "80"
}
