package httpapi

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

// Exercises actual TLS -> reverse proxy -> app sockets using the documented
// single-hop trust contract. It is not a claim about an installed nginx service.
func TestTrustedProxyHTTPSLifecycle(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	s.auth.options.SecureCookies = true
	backend := httptest.NewServer(s.Handler())
	defer backend.Close()
	var publicHost string
	proxy := httptest.NewUnstartedServer(&httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(p *httputil.ProxyRequest) {
			p.Out.URL.Scheme = "http"
			p.Out.URL.Host = backend.Listener.Addr().String()
			p.Out.Host = publicHost
			peer, _, _ := net.SplitHostPort(p.In.RemoteAddr)
			p.Out.Header.Set("X-Forwarded-For", peer)
			p.Out.Header.Set("X-Forwarded-Proto", "https")
			p.Out.Header.Set("X-Forwarded-Host", publicHost)
			p.Out.Header.Del("Forwarded")
			p.Out.Header.Del("X-Real-IP")
		},
	})
	publicHost = proxy.Listener.Addr().String()
	var err error
	s.networkPolicy, err = netaccess.Validate(netaccess.Config{Mode: netaccess.TrustedProxy, ListenAddress: backend.Listener.Addr().String(), PublicURL: "https://" + publicHost, TrustedProxies: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	proxy.StartTLS()
	defer proxy.Close()
	client := proxy.Client()
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	request := func(method, path, body string, cookie *http.Cookie) *http.Response {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, method, proxy.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", proxy.URL)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	closeProxyResponse(t, request(http.MethodGet, "/api/state", "", nil), http.StatusUnauthorized)
	login := request(http.MethodPost, "/api/auth/login", `{"username":"owner","password":"a long review passphrase"}`, nil)
	cookie := proxyLoginCookie(t, login)
	closeProxyResponse(t, login, http.StatusOK)
	closeProxyResponse(t, request(http.MethodGet, "/api/state", "", cookie), http.StatusOK)
	closeProxyResponse(t, request(http.MethodPost, "/api/host/path-picker", `{}`, cookie), http.StatusForbidden)
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, proxy.URL+"/api/auth/logout", nil)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://untrusted.example")
	wrongOrigin, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	closeProxyResponse(t, wrongOrigin, http.StatusForbidden)
	stream := request(http.MethodGet, "/api/motion/events?client_id=proxy-observer", "", cookie)
	defer func() { _ = stream.Body.Close() }()
	reader := bufio.NewReader(stream.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != "event: motion\n" {
		t.Fatalf("proxy SSE opening: %q %v", line, err)
	}
	closeProxyResponse(t, request(http.MethodPost, "/api/motion/stop", "", nil), http.StatusOK)
	closeProxyResponse(t, request(http.MethodPost, "/api/auth/logout", "", cookie), http.StatusNoContent)
	_, _ = io.Copy(io.Discard, reader)
	if ctx.Err() != nil {
		t.Fatal("logout did not close the existing proxied stream")
	}
	closeProxyResponse(t, request(http.MethodGet, "/api/state", "", cookie), http.StatusUnauthorized)
	t.Log("TLS proxy: login/cookie, protected state, local-only denial, foreign Origin denial, SSE, public Stop, logout and open-stream retirement passed")
}

func proxyLoginCookie(t *testing.T, login *http.Response) *http.Cookie {
	t.Helper()
	var cookie *http.Cookie
	for _, issued := range login.Cookies() {
		if issued.Name == secureSessionCookieName {
			cookie = issued
		}
	}
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" {
		t.Fatal("proxy login did not issue a secure, host-only, strict cookie")
	}
	return cookie
}

func closeProxyResponse(t *testing.T, r *http.Response, want int) {
	t.Helper()
	_, err := io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
	if err != nil || r.StatusCode != want {
		t.Fatalf("status=%d want=%d err=%v", r.StatusCode, want, err)
	}
}
