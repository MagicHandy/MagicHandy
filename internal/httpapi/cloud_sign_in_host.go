package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

type cloudSignInHost struct {
	Available bool   `json:"available"`
	URL       string `json:"url,omitempty"`
}

// The OAuth callback is loopback-only, so it must open on the app host.
// A browser on that host may use its LAN address (including a LAN-only HTTPS
// listener). Use socket facts, never DNS, forwarding headers, or a UI guess.
func chatGPTSignInHost(r *http.Request) cloudSignInHost {
	result := cloudSignInHost{Available: isLocalHostRequest(r)}
	local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return result
	}
	ip, port := endpointIP(local.String())
	if !ip.IsValid() || ip.IsUnspecified() || port == "" {
		return result
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	link := url.URL{Scheme: scheme, Host: net.JoinHostPort(ip.String(), port), Path: "/", Fragment: "/settings/chat/model"}
	result.URL = link.String()
	if netaccess.IsForwarded(r) || netaccess.HasForwardingHeaders(r) {
		result.Available = false
		return result
	}
	peer, _ := endpointIP(r.RemoteAddr)
	host, _ := endpointIP(r.Host)
	if peer.IsValid() && peer.Unmap() == ip.Unmap() && host.IsValid() && host.Unmap() == ip.Unmap() {
		result.Available = true
	}
	return result
}

func endpointIP(endpoint string) (netip.Addr, string) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		host, port = strings.Trim(endpoint, "[]"), ""
	}
	ip, _ := netip.ParseAddr(host)
	return ip, port
}

func (s *Server) cloudStatusForRequest(r *http.Request) map[string]any {
	status := s.cloudStatus(r.Context())
	status["sign_in_host"] = chatGPTSignInHost(r)
	return status
}
