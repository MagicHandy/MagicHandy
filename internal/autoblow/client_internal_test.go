package autoblow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnlyAutoblowClustersReceiveTheToken(t *testing.T) {
	for cluster, want := range map[string]string{
		"eu-central-1.autoblowapi.com": "https://eu-central-1.autoblowapi.com",
		"US-EAST-2.autoblowapi.com":    "https://us-east-2.autoblowapi.com",
	} {
		if got, err := cloudClusterURL(cluster); err != nil || got != want {
			t.Errorf("cloudClusterURL(%q) = %q, %v", cluster, got, err)
		}
	}
	for _, cluster := range []string{"", "evil.example.com", "autoblowapi.com.evil.com", "a.b.autoblowapi.com/x", "user@x.autoblowapi.com", "x.autoblowapi.com:8443", "eu_central.autoblowapi.com"} {
		if _, err := cloudClusterURL(cluster); !errors.Is(err, ErrBadCluster) {
			t.Errorf("cloudClusterURL(%q) = %v, want ErrBadCluster", cluster, err)
		}
	}
}

func TestLookupNamesTheOtherDevice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"connected":true,"cluster":"eu-central-1.autoblowapi.com","deviceType":"vacuglide"}`))
	}))
	defer server.Close()
	client, err := New(FamilyUltra, "token", Options{LatencyURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Lookup(context.Background())
	var wrong WrongDeviceError
	if !errors.As(err, &wrong) || wrong.Got != DeviceTypeVacuGlide {
		t.Fatalf("lookup = %v, want a wrong-device error", err)
	}
	if err.Error() != "this token belongs to a VacuGlide, not an Autoblow Ultra" {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestRateLimitHeadersAreRecorded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-ratelimit-limit", "100")
		w.Header().Set("x-ratelimit-remaining", "97")
		w.Header().Set("x-ratelimit-reset", "30")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"connected":true,"cluster":"eu-central-1.autoblowapi.com","deviceType":"autoblow-ultra"}`))
	}))
	defer server.Close()
	client, err := New(FamilyUltra, "token", Options{LatencyURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Lookup(context.Background()); err != nil {
		t.Fatal(err)
	}
	limit := client.RateLimit()
	if !limit.Known || limit.Limit != 100 || limit.Remaining != 97 || limit.Reset.IsZero() {
		t.Fatalf("rate limit = %+v", limit)
	}
}
