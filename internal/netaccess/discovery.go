package netaccess

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/mholt/acmez/v3/acme"
)

const letsEncryptDirectory = "https://acme-v02.api.letsencrypt.org/directory"

// InternetDiscovery describes observations, never a claim of inbound reachability.
type InternetDiscovery struct {
	PublicIP     string `json:"public_ip"`
	TermsURL     string `json:"terms_url"`
	IPError      bool   `json:"ip_error"`
	CAError      bool   `json:"ca_error"`
	ExternalPort int    `json:"external_port"`
}

func certificateHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Timeout: 15 * time.Second, Transport: boundedCertificateTransport{transport},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type boundedCertificateTransport struct{ http.RoundTripper }

func (t boundedCertificateTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.RoundTripper.RoundTrip(r)
	if err == nil {
		response.Body = &boundedCertificateBody{ReadCloser: response.Body, left: 1 << 20}
	}
	return response, err
}

type boundedCertificateBody struct {
	io.ReadCloser
	left int64
}

func (r *boundedCertificateBody) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, errors.New("certificate service response exceeded size limit")
	}
	if int64(len(p)) > r.left {
		p = p[:r.left]
	}
	n, err := r.ReadCloser.Read(p)
	r.left -= int64(n)
	return n, err
}

// DiscoverInternet contacts only fixed services, when explicitly requested from
// setup. No URL from a browser is fetched. Private worker addresses stay private.
func DiscoverInternet(ctx context.Context) InternetDiscovery {
	client := certificateHTTPClient()
	defer client.CloseIdleConnections()
	return discoverInternet(ctx, client, "https://api4.ipify.org", letsEncryptDirectory)
}

// Discover coalesces repeated UI clicks and bounds provider traffic per process.
func (a *Automation) Discover(ctx context.Context) (InternetDiscovery, error) {
	a.mu.Lock()
	if time.Since(a.discovered) < time.Minute {
		result := a.discovery
		a.mu.Unlock()
		return result, nil
	}
	if a.closed || a.discovering {
		a.mu.Unlock()
		return InternetDiscovery{}, errors.New("internet address detection is already running or unavailable")
	}
	a.discovering = true
	a.wg.Add(1)
	a.mu.Unlock()
	defer a.wg.Done()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(a.ctx, cancel)
	defer stop()
	result := DiscoverInternet(ctx)
	a.mu.Lock()
	a.discovery, a.discovered, a.discovering = result, time.Now(), false
	a.mu.Unlock()
	return result, nil
}

func discoverInternet(ctx context.Context, client *http.Client, ipURL, directory string) InternetDiscovery {
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	result := InternetDiscovery{ExternalPort: 443}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		result.PublicIP, result.IPError = detectPublicIP(ctx, client, ipURL)
	}()
	go func() {
		defer wg.Done()
		ca := &acme.Client{Directory: directory, HTTPClient: client}
		dir, err := ca.GetDirectory(ctx)
		if err != nil || dir.Meta.TermsOfService == "" || dir.Meta.Profiles["shortlived"] == "" {
			result.CAError = true
			return
		}
		result.TermsURL = dir.Meta.TermsOfService
	}()
	wg.Wait()
	return result
}

func detectPublicIP(ctx context.Context, client *http.Client, endpoint string) (string, bool) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", true
	}
	response, err := client.Do(request)
	if err != nil {
		return "", true
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, 128))
	ip, parseErr := netip.ParseAddr(strings.TrimSpace(string(data)))
	if err != nil || response.StatusCode != http.StatusOK || len(data) == 128 || parseErr != nil || !publicIP(ip) {
		return "", true
	}
	return ip.String(), false
}
