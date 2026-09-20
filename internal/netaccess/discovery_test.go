package netaccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublicIPDiscoveryRejectsNonPublicAndOversizedResponses(t *testing.T) {
	for _, value := range []string{"192.168.1.2", "127.0.0.1", "100.64.1.2", "::1", "::ffff:8.8.8.8", "2001:db8::1", strings.Repeat("8", 128), "<html>error</html>"} {
		t.Run(value[:min(len(value), 24)], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(value)) }))
			defer server.Close()
			if ip, failed := detectPublicIP(t.Context(), server.Client(), server.URL); !failed || ip != "" {
				t.Fatal("accepted invalid external address")
			}
		})
	}
}

func TestInternetDiscoveryIsBoundedAndDoesNotClaimReachability(t *testing.T) {
	policy := managedPolicy(t, AutomaticPublic)
	ca := newCertificateCA(t, policy)
	address := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("8.8.4.4\n")) }))
	defer address.Close()
	result := discoverInternet(t.Context(), address.Client(), address.URL, ca.server.URL+"/directory")
	if result.PublicIP != "8.8.4.4" || result.TermsURL != testTerms || result.ExternalPort != 443 || result.CAError || result.IPError {
		t.Fatalf("bad discovery: %+v", result)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	result = discoverInternet(ctx, slow.Client(), slow.URL, slow.URL+"/directory")
	if !result.IPError || !result.CAError || time.Since(start) > time.Second {
		t.Fatal("discovery ignored cancellation")
	}
}
