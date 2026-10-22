package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// browserPageLoad models a link from another site or another MagicHandy port.
func browserPageLoad(method, host, path, site string) *http.Request {
	r := httptest.NewRequest(method, "http://"+host+path, nil)
	r.Header.Set("Sec-Fetch-Site", site)
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	r.Header.Set("Sec-Fetch-Dest", "document")
	return r
}

func TestBrowserPageLinksOpenButAPIsStaySameOrigin(t *testing.T) {
	local := newTestServer(t)
	public, _ := newAuthenticationTestServer(t, true, true, []string{"app.example.test"})
	for name, target := range map[string]struct {
		host    string
		handler http.Handler
	}{
		"local app":     {"127.0.0.1:49717", local.Handler()},
		"local remote":  {"127.0.0.1:49718", local.RemoteHandler(RemoteInterfaceOptions{})},
		"public app":    {"app.example.test:8443", public.Handler()},
		"public remote": {"app.example.test:8444", public.RemoteHandler(RemoteInterfaceOptions{AllowedBrowserHosts: []string{"app.example.test"}, SecureCookies: true})},
	} {
		t.Run(name, func(t *testing.T) {
			for _, site := range []string{"same-site", "cross-site"} {
				t.Run(site, func(t *testing.T) {
					for _, method := range []string{http.MethodGet, http.MethodHead} {
						for _, path := range []string{"/", "/settings/access/network"} {
							t.Run(method+path, func(t *testing.T) {
								w := httptest.NewRecorder()
								target.handler.ServeHTTP(w, browserPageLoad(method, target.host, path, site))
								if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
									t.Fatalf("page navigation: %d %q %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
								}
							})
						}
					}
					for name, request := range map[string]*http.Request{
						"API root":        browserPageLoad(http.MethodGet, target.host, "/api", site),
						"API prefix":      browserPageLoad(http.MethodGet, target.host, "/api/", site),
						"API page load":   browserPageLoad(http.MethodGet, target.host, "/api/auth/status", site),
						"private API":     browserPageLoad(http.MethodGet, target.host, "/api/settings", site),
						"API HEAD":        browserPageLoad(http.MethodHead, target.host, "/api/state", site),
						"login post":      browserPageLoad(http.MethodPost, target.host, "/api/auth/login", site),
						"form post":       browserPageLoad(http.MethodPost, target.host, "/", site),
						"settings write":  browserPageLoad(http.MethodPut, target.host, "/api/settings", site),
						"foreign Host":    browserPageLoad(http.MethodGet, "attacker.example:8443", "/", site),
						"script fetch":    browserPageLoad(http.MethodGet, target.host, "/", site),
						"frame":           browserPageLoad(http.MethodGet, target.host, "/", site),
						"object":          browserPageLoad(http.MethodGet, target.host, "/", site),
						"missing context": browserPageLoad(http.MethodGet, target.host, "/", site),
						"with an Origin":  browserPageLoad(http.MethodGet, target.host, "/", site),
					} {
						t.Run(name, func(t *testing.T) {
							switch name {
							case "script fetch":
								request.Header.Set("Sec-Fetch-Mode", "cors")
								request.Header.Set("Sec-Fetch-Dest", "empty")
							case "frame":
								request.Header.Set("Sec-Fetch-Dest", "iframe")
							case "object":
								request.Header.Set("Sec-Fetch-Dest", "object")
							case "missing context":
								request.Header.Del("Sec-Fetch-Mode")
							case "with an Origin":
								request.Header.Set("Origin", "https://untrusted.example")
							}
							w := httptest.NewRecorder()
							target.handler.ServeHTTP(w, request)
							if w.Code != http.StatusForbidden {
								t.Fatalf("status %d, want %d: %s", w.Code, http.StatusForbidden, w.Body.String())
							}
						})
					}
				})
			}
			if strings.HasPrefix(name, "public") {
				w := httptest.NewRecorder()
				target.handler.ServeHTTP(w, browserPageLoad(http.MethodGet, target.host, "/api/settings", "same-origin"))
				if w.Code != http.StatusUnauthorized {
					t.Fatalf("same-origin private API without login: %d %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestBrowserBoundaryVariesOnRequestContext(t *testing.T) {
	handler := protectBrowserRequests(nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, mode := range []string{"navigate", "cors"} {
		t.Run(mode, func(t *testing.T) {
			request := browserPageLoad(http.MethodGet, "127.0.0.1:49717", "/", "cross-site")
			request.Header.Set("Sec-Fetch-Mode", mode)
			w := httptest.NewRecorder()
			w.Header().Add("Vary", "Accept-Encoding")
			handler.ServeHTTP(w, request)
			vary := strings.Join(w.Header().Values("Vary"), ", ")
			for _, header := range []string{"Accept-Encoding", "Origin", "Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest"} {
				if !strings.Contains(vary, header) {
					t.Errorf("Vary %q omits %s", vary, header)
				}
			}
		})
	}
}
