package autoblow_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
	"github.com/mapledaemon/MagicHandy/internal/autoblowsim"
)

const secretToken = "tok-secret-1234"

func simulatedClient(t *testing.T, family string) (*autoblow.Client, *autoblowsim.Server) {
	t.Helper()
	sim, err := autoblowsim.StartWith(autoblowsim.Options{Heartbeat: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sim.Close() })
	client, err := autoblow.New(family, secretToken, autoblow.Options{LatencyURL: sim.URL(), ClusterURL: sim.ClusterURL})
	if err != nil {
		t.Fatal(err)
	}
	return client, sim
}

func TestLookupBindsTheClusterAndChecksTheDeviceType(t *testing.T) {
	ctx := context.Background()
	client, _ := simulatedClient(t, autoblow.FamilyVacuGlide)
	if _, err := client.VacuGlideState(ctx); !errors.Is(err, autoblow.ErrNotResolved) {
		t.Fatalf("state before lookup = %v, want ErrNotResolved", err)
	}
	connection, err := client.Lookup(ctx)
	if err != nil || !connection.Connected || connection.DeviceType != autoblow.DeviceTypeVacuGlide {
		t.Fatalf("lookup = %+v, %v", connection, err)
	}
	info, err := client.Info(ctx)
	if err != nil || info.FirmwareStatus != "UP_TO_DATE" {
		t.Fatalf("info = %+v, %v", info, err)
	}
}

func TestLookupRejectsOfflineDevices(t *testing.T) {
	client, sim := simulatedClient(t, autoblow.FamilyUltra)
	sim.SetOnline("autoblow", false)
	if _, err := client.Lookup(context.Background()); !errors.Is(err, autoblow.ErrOffline) {
		t.Fatalf("offline lookup = %v, want ErrOffline", err)
	}
}

func TestUltraCommandsUseTheDocumentedShapes(t *testing.T) {
	ctx := context.Background()
	client, sim := simulatedClient(t, autoblow.FamilyUltra)
	if _, err := client.Lookup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Goto(ctx, 80.4, 49.6); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Oscillate(ctx, 20, 30, 70); err != nil {
		t.Fatal(err)
	}
	state, err := client.StopUltra(ctx)
	if err != nil || state.OperationalMode != autoblow.ModeOscillatorPaused {
		t.Fatalf("stop = %+v, %v", state, err)
	}
	requests := sim.Requests()
	if len(requests) != 3 || requests[0].Path != "goto" || requests[0].Body["position"] != 80.0 || requests[0].Body["speed"] != 50.0 {
		t.Fatalf("requests = %+v", requests)
	}
	if requests[2].Path != "oscillate/stop" || len(requests[2].Body) != 0 {
		t.Fatalf("stop request = %+v, want no body", requests[2])
	}
}

func TestVacuGlideCommandsUseTheDocumentedShapes(t *testing.T) {
	ctx := context.Background()
	client, sim := simulatedClient(t, autoblow.FamilyVacuGlide)
	if _, err := client.Lookup(ctx); err != nil {
		t.Fatal(err)
	}
	if state, err := client.TargetSpeed(ctx, 42); err != nil || state.TargetSpeed != 42 || state.OperationalMode != autoblow.ModeTargetSpeedPlaying {
		t.Fatalf("target speed = %+v, %v", state, err)
	}
	if state, err := client.Valve(ctx, autoblow.ValveStrokePlus, true); err != nil || !state.StrokePlusValve {
		t.Fatalf("valve = %+v, %v", state, err)
	}
	if state, err := client.Valve(ctx, autoblow.ValveStrokePlus, false); err != nil || state.StrokePlusValve {
		t.Fatalf("valve close = %+v, %v", state, err)
	}
	if state, err := client.StopVacuGlide(ctx); err != nil || state.TargetSpeed != 0 {
		t.Fatalf("stop = %+v, %v", state, err)
	}
	if sim.VacuGlideSpeed() != 0 {
		t.Fatal("the simulated motor still runs after Stop")
	}
}

func TestStatusErrorsMapAndNeverCarryTheToken(t *testing.T) {
	ctx := context.Background()
	client, sim := simulatedClient(t, autoblow.FamilyVacuGlide)
	if _, err := client.Lookup(ctx); err != nil {
		t.Fatal(err)
	}
	sim.FailNext("vacuglide", http.StatusTooManyRequests, 1)
	if _, err := client.TargetSpeed(ctx, 10); !errors.Is(err, autoblow.ErrRateLimited) {
		t.Fatalf("429 = %v", err)
	}
	sim.FailNext("vacuglide", http.StatusUnauthorized, 1)
	if _, err := client.TargetSpeed(ctx, 10); !errors.Is(err, autoblow.ErrUnauthorized) {
		t.Fatalf("401 = %v", err)
	}
	sim.SetOnline("vacuglide", false)
	_, err := client.TargetSpeed(ctx, 10)
	if !errors.Is(err, autoblow.ErrOffline) {
		t.Fatalf("502 = %v", err)
	}
	sim.SetOnline("vacuglide", true)
	var refused *autoblow.APIError
	if _, err := client.Valve(ctx, "bogus", true); !errors.As(err, &refused) || refused.Status != http.StatusNotFound || strings.Contains(err.Error(), secretToken) {
		t.Fatalf("refused command = %v", err)
	}
	unreachable, _ := autoblow.New(autoblow.FamilyVacuGlide, secretToken, autoblow.Options{LatencyURL: "http://127.0.0.1:1"})
	if _, err := unreachable.Lookup(ctx); err == nil || strings.Contains(err.Error(), secretToken) {
		t.Fatalf("unreachable lookup = %v, must fail without the token", err)
	}
}

func TestEventsCarryButtonPressesAndHeartbeats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, sim := simulatedClient(t, autoblow.FamilyVacuGlide)
	if _, err := client.Lookup(ctx); err != nil {
		t.Fatal(err)
	}
	stream, err := client.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	first, err := stream.Next()
	if err != nil || first.Name != autoblow.EventConnected {
		t.Fatalf("first event = %+v, %v", first, err)
	}
	sim.Press("vacuglide", autoblow.ButtonVacuGlideMode)
	for {
		event, err := stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if event.Name == autoblow.ButtonVacuGlideMode {
			return
		}
		if event.Name != autoblow.EventHeartbeat {
			t.Fatalf("unexpected event %+v", event)
		}
	}
}

func TestTokensMustLookLikeTokens(t *testing.T) {
	for _, token := range []string{"", " ", "has space", "tab\tinside", "ünïcode", strings.Repeat("a", 257)} {
		if autoblow.ValidToken(token) {
			t.Errorf("ValidToken(%q) = true", token)
		}
	}
	if !autoblow.ValidToken("7a93bfc5-4043-4ade-aee5-bec8012b0e12") {
		t.Error("a UUID token was rejected")
	}
	if _, err := autoblow.New(autoblow.FamilyUltra, "  ", autoblow.Options{}); !errors.Is(err, autoblow.ErrNoToken) {
		t.Errorf("blank token = %v", err)
	}
}
