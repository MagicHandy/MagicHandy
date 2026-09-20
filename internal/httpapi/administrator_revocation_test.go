package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

type admittedAdministratorBody struct {
	io.ReadCloser
	once    sync.Once
	entered chan struct{}
}

func (b *admittedAdministratorBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	return b.ReadCloser.Read(p)
}

// Hold the body of an authenticated request over a real TLS connection, then
// disable its administrator through a second login. Both protocols must abort
// old work and leave no replacement account, re-enablement, grant or config.
func TestDisabledAdministratorCannotFinishHTTPSWrite(t *testing.T) {
	for _, useHTTP2 := range []bool{false, true} {
		for _, action := range []string{"create", "enable", "grant", "network"} {
			t.Run(fmt.Sprintf("http2=%t/%s", useHTTP2, action), func(t *testing.T) {
				s, store, first, firstCookie := newControllerSessionFixture(t)
				second, err := store.Create(t.Context(), "other-owner", "another long review passphrase", accounts.RoleAdmin)
				if err != nil {
					t.Fatal(err)
				}
				token, _, err := store.NewSession(t.Context(), second.ID)
				if err != nil {
					t.Fatal(err)
				}
				target, err := store.Create(t.Context(), "observer", "a long observer passphrase", accounts.RoleOperator)
				if err != nil {
					t.Fatal(err)
				}
				secondCookie := &http.Cookie{Name: secureSessionCookieName, Value: token, Path: "/", Secure: true,
					HttpOnly: true, SameSite: http.SameSiteStrictMode}
				s.auth.options.SecureCookies = true
				firstCookie.Name, secondCookie.Name = secureSessionCookieName, secureSessionCookieName
				entered, handled := make(chan struct{}), make(chan struct{})
				var admittedProtocol int
				host := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("X-Test-Delay") == "1" {
						admittedProtocol = r.ProtoMajor
						r.Body = &admittedAdministratorBody{ReadCloser: r.Body, entered: entered}
						defer close(handled)
					}
					s.Handler().ServeHTTP(w, r)
				}))
				host.EnableHTTP2 = useHTTP2
				s.networkPolicy, err = netaccess.Validate(netaccess.Config{Mode: netaccess.DirectHTTPS,
					ListenAddress: host.Listener.Addr().String(), PublicURL: "https://" + host.Listener.Addr().String(),
					TLSCertificate: "test-certificate", TLSPrivateKey: "test-key"})
				if err != nil {
					t.Fatal(err)
				}
				host.StartTLS()
				defer host.Close()
				client := host.Client()
				client.Timeout = 5 * time.Second
				ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
				defer cancel()
				method, path, body := delayedAdministratorMutation(action, first.ID, target.ID)
				reader, writer := io.Pipe()
				defer func() { _ = reader.Close() }()
				defer func() { _ = writer.Close() }()
				r, err := http.NewRequestWithContext(ctx, method, host.URL+path, reader)
				if err != nil {
					t.Fatal(err)
				}
				r.AddCookie(firstCookie)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Origin", host.URL)
				r.Header.Set("X-Test-Delay", "1")
				finished := make(chan int, 1)
				go func() {
					response, err := client.Do(r)
					if err != nil {
						finished <- 0 // Revocation may abort the connection/stream.
						return
					}
					_, _ = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
					finished <- response.StatusCode
				}()
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("request did not reach authenticated body read")
				}
				if (admittedProtocol == 2) != useHTTP2 {
					t.Fatalf("unexpected protocol %d", admittedProtocol)
				}
				if status := administratorHTTPSRequest(ctx, t, client, host.URL, secondCookie, http.MethodPut,
					"/api/accounts/"+first.ID+"/disabled", `{"disabled":true}`); status != http.StatusNoContent {
					t.Fatalf("disable status=%d", status)
				}
				_, _ = io.WriteString(writer, body)
				_ = writer.Close()
				select {
				case <-handled:
				case <-ctx.Done():
					t.Fatal("disabled request handler did not retire")
				}
				if status := <-finished; status >= 200 && status < 300 {
					t.Fatalf("disabled request succeeded: %d", status)
				}
				assertDisabledAdministratorWritesAbsent(t, s, first.ID)
				if status := administratorHTTPSRequest(ctx, t, client, host.URL, firstCookie, http.MethodGet, "/api/accounts", ""); status != http.StatusUnauthorized {
					t.Fatalf("disabled cookie accepted: %d", status)
				}
			})
		}
	}
}

func delayedAdministratorMutation(action, adminID, operatorID string) (method, path, body string) {
	switch action {
	case "enable":
		return http.MethodPut, "/api/accounts/" + adminID + "/disabled", `{"disabled":false}`
	case "grant":
		return http.MethodPut, "/api/accounts/" + operatorID + "/control-grant", `{"permanent":true}`
	case "network":
		return http.MethodPut, "/api/network", `{"password":"a long review passphrase","config":{"mode":"local","listen_address":"127.0.0.1:49717"}}`
	default:
		return http.MethodPost, "/api/accounts", `{"username":"replacement-owner","password":"independent long passphrase","role":"admin"}`
	}
}

func assertDisabledAdministratorWritesAbsent(t *testing.T, s *Server, accountID string) {
	t.Helper()
	listed, err := s.accounts.List(t.Context())
	if err != nil || len(listed) != 3 {
		t.Fatalf("replacement account persisted: %d %v", len(listed), err)
	}
	for _, account := range listed {
		if account.ID == accountID && !account.Disabled {
			t.Fatal("disabled account re-enabled itself")
		}
	}
	// Read the durable row: ControlGrant hides a disabled issuer's grant,
	// but that row would revive when another administrator enables the issuer.
	var grants int
	if err := s.store.Datastore().SQL().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM user_control_grants`).Scan(&grants); err != nil || grants != 0 {
		t.Fatal("permission persisted after revocation", grants, err)
	}
	config, err := netaccess.Load(t.Context(), s.store.Datastore())
	if err != nil || config != nil {
		t.Fatal("network configuration persisted after revocation", err)
	}
}

func administratorHTTPSRequest(ctx context.Context, t *testing.T, client *http.Client, base string, cookie *http.Cookie, method, path, body string) int {
	t.Helper()
	r, err := http.NewRequestWithContext(ctx, method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.AddCookie(cookie)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", base)
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return response.StatusCode
}
