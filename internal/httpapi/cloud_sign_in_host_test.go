package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

func TestChatGPTSignInUsesLocalSocketIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, local, peer, host string
		forwarded, want         bool
	}{
		{"loopback", "127.0.0.1:49717", "127.0.0.1:51000", "localhost:49717", false, true},
		{"host LAN browser", "192.168.0.12:49717", "192.168.0.12:51000", "192.168.0.12:49717", false, true},
		{"another LAN device", "192.168.0.12:49717", "192.168.0.50:51000", "192.168.0.12:49717", false, false},
		{"public hostname", "192.168.0.12:49717", "192.168.0.12:51000", "example.test:8443", false, false},
		{"forwarded LAN", "192.168.0.12:49717", "192.168.0.12:51000", "192.168.0.12:49717", true, false},
		{"forwarded loopback", "127.0.0.1:49717", "127.0.0.1:51000", "localhost:49717", true, false},
		{"IPv6 host", "[fd00::12]:49717", "[fd00::12]:51000", "[fd00::12]:49717", false, true},
		{"unspecified bind", "0.0.0.0:49717", "192.168.0.12:51000", "192.168.0.12:49717", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/api/llm/cloud", nil)
			r.RemoteAddr = tc.peer
			local, err := net.ResolveTCPAddr("tcp", tc.local)
			if err != nil {
				t.Fatal(err)
			}
			r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, local))
			if tc.forwarded {
				r.Header.Set("X-Forwarded-For", "127.0.0.1")
			}
			got := chatGPTSignInHost(r)
			if got.Available != tc.want {
				t.Fatalf("host admission=%+v, want available=%v", got, tc.want)
			}
			if strings.Contains(got.URL, "example.test") {
				t.Fatal("local link trusted the requested Host header")
			}
			if tc.name == "host LAN browser" && got.URL != "https://192.168.0.12:49717/#/settings/chat/model" {
				t.Fatalf("local link=%q", got.URL)
			}
		})
	}
}

func TestChatGPTSignInRejectsTrustedProxyLocalIdentity(t *testing.T) {
	policy, err := netaccess.Validate(netaccess.Config{Mode: netaccess.TrustedProxy, ListenAddress: "127.0.0.1:49717",
		PublicURL: "https://localhost", TrustedProxies: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "http://localhost/api/llm/cloud", nil)
	r.RemoteAddr = "127.0.0.1:51000"
	r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 49717}))
	r.Header.Set("Origin", "https://localhost")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "localhost")
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	accepted, err := policy.Accept(r)
	if err != nil {
		t.Fatal(err)
	}
	if chatGPTSignInHost(accepted).Available {
		t.Fatal("proxy traffic became a host browser")
	}
}

func TestChatGPTSignInLANHostReachesOAuthButRemoteDoesNot(t *testing.T) {
	var contacted atomic.Int32
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		contacted.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer issuer.Close()
	s := newCloudTestServer(t, Runtime{OpenAIIssuer: issuer.URL})
	for _, remote := range []bool{true, false} {
		r := withController(httptest.NewRequest(http.MethodPost, "https://192.168.0.12:49717/api/llm/cloud/sign-in", strings.NewReader(`{}`)))
		r.RemoteAddr = "192.168.0.12:51000"
		if remote {
			r.RemoteAddr = "192.168.0.50:51000"
		}
		r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("192.168.0.12"), Port: 49717}))
		w := httptest.NewRecorder()
		s.handleCloudSignIn(w, r)
		if remote && (w.Code != http.StatusConflict || contacted.Load() != 0) {
			t.Fatalf("remote sign-in=%d, contacted=%d", w.Code, contacted.Load())
		}
		if !remote && (w.Code != http.StatusBadGateway || contacted.Load() == 0) {
			t.Fatalf("local sign-in did not reach the fixture: %d %s", w.Code, w.Body.String())
		}
	}
}
