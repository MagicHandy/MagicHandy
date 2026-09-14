package netaccess

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func proxyPolicy(t *testing.T) *Policy {
	t.Helper()
	policy, err := Validate(Config{Mode: TrustedProxy, ListenAddress: "127.0.0.1:49717", PublicURL: "https://control.example.test",
		TrustedProxies: []string{"127.0.0.1/32", "10.10.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func proxyRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://control.example.test/api/state", nil)
	r.RemoteAddr = "127.0.0.1:34567"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "control.example.test")
	r.Header.Set("X-Forwarded-For", "192.0.2.4")
	return r
}

func TestProxyTrustUsesActualPeerAndSingleReplacedHeaders(t *testing.T) {
	policy := proxyPolicy(t)
	accepted, err := policy.Accept(proxyRequest())
	if err != nil || !IsForwarded(accepted) || ClientIP(accepted) != "192.0.2.4" || accepted.RemoteAddr != "127.0.0.1:34567" || accepted.TLS != nil {
		t.Fatalf("accepted proxy request lost trust separation: %v %v", accepted, err)
	}
	for name, change := range map[string]func(*http.Request){
		"untrusted peer":       func(r *http.Request) { r.RemoteAddr = "192.0.2.5:5000" },
		"wrong socket Host":    func(r *http.Request) { r.Host = "127.0.0.1:49717" },
		"wrong forwarded Host": func(r *http.Request) { r.Header.Set("X-Forwarded-Host", "attacker.test") },
		"wrong port":           func(r *http.Request) { r.Host = "control.example.test:444" },
		"insecure scheme":      func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "http") },
		"scheme chain":         func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https,http") },
		"address chain":        func(r *http.Request) { r.Header.Set("X-Forwarded-For", "127.0.0.1, 192.0.2.4") },
		"duplicate address":    func(r *http.Request) { r.Header.Add("X-Forwarded-For", "127.0.0.1") },
		"missing address":      func(r *http.Request) { r.Header.Del("X-Forwarded-For") },
		"conflicting family":   func(r *http.Request) { r.Header.Set("Forwarded", "for=127.0.0.1;proto=https") },
	} {
		t.Run(name, func(t *testing.T) {
			r := proxyRequest()
			change(r)
			if _, err := policy.Accept(r); err == nil {
				t.Fatal("unsafe proxy request accepted")
			}
		})
	}
}

func TestDirectAndLocalModesDoNotTrustForwardingHeaders(t *testing.T) {
	for _, mode := range []string{Local, DirectHTTPS} {
		config := Config{Mode: mode, ListenAddress: "127.0.0.1:49717"}
		if mode == DirectHTTPS {
			config.PublicURL, config.TLSCertificate, config.TLSPrivateKey = "https://control.example.test", "cert.pem", "key.pem"
		}
		policy, err := Validate(config)
		if err != nil {
			t.Fatal(err)
		}
		r := proxyRequest()
		r.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
		if _, err := policy.Accept(r); err == nil {
			t.Fatalf("%s accepted forwarding headers", mode)
		}
		r.Header = make(http.Header)
		if _, err := policy.Accept(r); err != nil {
			t.Fatalf("%s rejected ordinary request: %v", mode, err)
		}
		if mode == DirectHTTPS {
			r.TLS = nil
			if _, err := policy.Accept(r); err == nil {
				t.Fatal("direct mode accepted plaintext")
			}
		}
	}
}

func TestRemotePolicyValidationFailsClosed(t *testing.T) {
	for _, config := range []Config{
		{Mode: Local, ListenAddress: "0.0.0.0:49717"},
		{Mode: DirectHTTPS, ListenAddress: "0.0.0.0:49717", PublicURL: "http://control.test"},
		{Mode: DirectHTTPS, ListenAddress: "0.0.0.0:49717", PublicURL: "https://control.test", TLSCertificate: "cert"},
		{Mode: TrustedProxy, ListenAddress: "0.0.0.0:49717", PublicURL: "https://control.test", TrustedProxies: []string{"127.0.0.1"}},
		{Mode: TrustedProxy, ListenAddress: "127.0.0.1:49717", PublicURL: "https://control.test", TrustedProxies: []string{"0.0.0.0/0"}},
		{Mode: TrustedProxy, ListenAddress: "127.0.0.1:49717", PublicURL: "https://control.test"},
	} {
		if _, err := Validate(config); err == nil {
			t.Fatalf("unsafe policy accepted: %+v", config)
		}
	}
	for _, value := range []string{"http://control.test", "https://user:pass@control.test", "https://control.test/path", "https://control.test?", "https://control.test#tab", "https://*.test", "https://0.0.0.0", "https://control.test:0"} {
		if _, err := ParseOrigin(value); err == nil {
			t.Fatalf("unsafe origin accepted: %s", value)
		}
	}
}
