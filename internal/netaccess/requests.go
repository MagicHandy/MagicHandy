package netaccess

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type forwardedContextKey struct{}
type forwardedRequest struct{ clientIP string }

// IsForwarded preserves the trust boundary independently of a claimed client IP.
func IsForwarded(r *http.Request) bool {
	_, ok := r.Context().Value(forwardedContextKey{}).(forwardedRequest)
	return ok
}

// ClientIP is for throttling and diagnostics, never local privilege decisions.
func ClientIP(r *http.Request) string {
	if forwarded, ok := r.Context().Value(forwardedContextKey{}).(forwardedRequest); ok {
		return forwarded.clientIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// HasForwardingHeaders also blocks accidental forwarding to local-only routes.
func HasForwardingHeaders(r *http.Request) bool {
	for key := range r.Header {
		name := strings.ToLower(key)
		if name == "forwarded" || strings.HasPrefix(name, "x-forwarded-") || name == "x-real-ip" {
			return true
		}
	}
	return false
}

// Accept checks the actual peer before interpreting any proxy metadata.
// It never rewrites RemoteAddr or fakes a TLS connection state.
func (p *Policy) Accept(r *http.Request) (*http.Request, error) {
	if p.Config.Mode == Local {
		if HasForwardingHeaders(r) {
			return nil, errors.New("forwarded traffic is disabled in local mode")
		}
		return r, nil
	}
	origin, err := ParseOrigin("https://" + r.Host)
	if err != nil || origin.Host != p.Origin.Host {
		return nil, errors.New("request Host must match the configured public HTTPS origin")
	}
	if p.Config.Mode == DirectHTTPS {
		if r.TLS == nil || HasForwardingHeaders(r) {
			return nil, errors.New("direct HTTPS requires TLS and does not accept forwarding headers")
		}
		if p.Config.Scope == "lan" && !localPeer(ClientIP(r)) {
			return nil, errors.New("LAN access accepts only private or loopback client addresses")
		}
		return r, nil
	}
	return p.acceptProxy(r)
}

func (p *Policy) acceptProxy(r *http.Request) (*http.Request, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip, ipErr := netip.ParseAddr(host)
	trusted := false
	for _, peer := range p.Peers {
		trusted = trusted || (err == nil && ipErr == nil && peer.Contains(ip.Unmap()))
	}
	if !trusted {
		return nil, errors.New("request did not arrive from a configured trusted proxy")
	}
	if r.Header.Get("Forwarded") != "" || r.Header.Get("X-Real-IP") != "" ||
		singleHeader(r, "X-Forwarded-Proto") != "https" {
		return nil, errors.New("proxy must replace forwarding headers and declare one HTTPS scheme")
	}
	origin, err := ParseOrigin("https://" + singleHeader(r, "X-Forwarded-Host"))
	if err != nil || origin.Host != p.Origin.Host {
		return nil, errors.New("proxy host does not match the configured public HTTPS origin")
	}
	client, err := netip.ParseAddr(singleHeader(r, "X-Forwarded-For"))
	if err != nil || client.Zone() != "" || client.IsUnspecified() || client.IsMulticast() {
		return nil, errors.New("proxy must provide one client IP, replacing any incoming forwarded chain")
	}
	if p.Config.Scope == "lan" && !localPeer(client.String()) {
		return nil, errors.New("LAN access accepts only private or loopback client addresses")
	}
	ctx := context.WithValue(r.Context(), forwardedContextKey{}, forwardedRequest{clientIP: client.Unmap().String()})
	return r.WithContext(ctx), nil
}

func localPeer(value string) bool {
	ip, err := netip.ParseAddr(value)
	return err == nil && (ip.Unmap().IsPrivate() || ip.IsLoopback())
}

func singleHeader(r *http.Request, name string) string {
	values := r.Header.Values(name)
	if len(values) != 1 || strings.Contains(values[0], ",") {
		return ""
	}
	return strings.TrimSpace(values[0])
}
