package transport

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

// manualClock is advanced explicitly; the pacer's ticker is disabled and the
// tests call paceOnce, so every packet is deterministic.
type manualClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

type coyoteFixture struct {
	t         *testing.T
	owner     *Coyote
	connector *estim.SimulatedConnector
	clock     *manualClock
}

func newCoyoteFixture(t *testing.T) *coyoteFixture {
	t.Helper()
	fixture := &coyoteFixture{
		t:         t,
		connector: estim.NewSimulatedConnector(),
		clock:     &manualClock{now: time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC)},
	}
	fixture.owner = NewCoyote(CoyoteOptions{Connector: fixture.connector, Clock: fixture.clock.Now, Tick: time.Hour})
	if err := fixture.owner.Configure(CoyoteConfig{
		Limits:  estim.Limits{StrengthA: 20, StrengthB: 20, FrequencyBalanceA: 160, FrequencyBalanceB: 160},
		Profile: estim.Profile{Follow: estim.FollowStroke, MinHz: 40, MaxHz: 100},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.owner.Close() })
	return fixture
}

func (f *coyoteFixture) connect() *estim.SimulatedLink {
	f.t.Helper()
	if _, err := f.owner.Connect(context.Background()); err != nil {
		f.t.Fatal(err)
	}
	return f.connector.LastLink()
}

// confirm advances past the B1 wait window. The acknowledged zero write
// established a commanded baseline, distinct from a device report.
func (f *coyoteFixture) confirm() {
	f.t.Helper()
	f.clock.Advance(1100 * time.Millisecond)
	f.owner.paceOnce()
	f.waitFor("strength confirmed", func(status CoyoteStatus) bool { return status.StrengthKnown })
}

func (f *coyoteFixture) waitFor(what string, done func(CoyoteStatus) bool) CoyoteStatus {
	f.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		status := f.owner.Status()
		if done(status) {
			return status
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting for %s: %+v", what, status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *coyoteFixture) pace(packets int) {
	for range packets {
		f.clock.Advance(coyotePacketInterval)
		f.owner.paceOnce()
	}
}

func strokeCommand(stream string, durationMillis int64) AppendPointsCommand {
	command := AppendPointsCommand{StreamID: stream}
	for at := int64(0); at <= durationMillis; at += 250 {
		position := 0.0
		if (at/250)%2 == 1 {
			position = 100
		}
		command.Points = append(command.Points, TimedPoint{TimeMillis: at, PositionPercent: position})
	}
	return command
}

func TestCoyoteConnectWritesCeilingsAndStartsDisarmed(t *testing.T) {
	fixture := newCoyoteFixture(t)
	fixture.connector.StartAt(35, 12)
	link := fixture.connect()
	state := link.Snapshot()
	if state.Limits != 1 || state.CeilingA != 20 || state.CeilingB != 20 {
		t.Fatalf("connect must write the ceilings first: %+v", state)
	}
	if state.StrengthA != 0 || state.StrengthB != 0 {
		t.Fatalf("a new connection must start at zero strength: %+v", state)
	}
	if !state.LastWill {
		t.Fatal("connect must store a zero last-will packet")
	}
	// The ceiling write clamps A to 20 and reports that first; wait for the
	// zero's own report rather than any known strength.
	status := fixture.waitFor("zero report", func(status CoyoteStatus) bool {
		return status.StrengthKnown && !status.StrengthPending
	})
	if status.StrengthA != 0 || status.StrengthB != 0 || !status.Connected || !status.Simulated {
		t.Fatalf("status after connect = %+v", status)
	}
	if status.BatteryPercent == nil {
		t.Fatal("battery level was not read")
	}
}

func TestCoyoteStrengthStepsAreUserOnlyAndCapped(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	if _, err := fixture.owner.AdjustStrength(estim.ChannelA, 1); !errors.Is(err, estim.ErrStrengthUnconfirmed) {
		t.Fatalf("raise before confirmation = %v", err)
	}
	fixture.confirm()
	if status, err := fixture.owner.AdjustStrength(estim.ChannelA, 3); err != nil || status.ReferencePulse {
		t.Fatalf("raise while idle = %+v, %v; strength alone must not play", status, err)
	}
	fixture.pace(1)
	fixture.waitFor("raise report", func(status CoyoteStatus) bool { return status.StrengthA == 3 })
	if state := link.Snapshot(); state.StrengthA != 3 || state.OutputA != 0 || state.OutputB != 0 {
		t.Fatalf("changing strength must stay silent: %+v", state)
	}
	if _, err := fixture.owner.TestChannel(context.Background(), fixture.owner.ControlGeneration(), estim.ChannelA); err != nil {
		t.Fatal(err)
	}
	fixture.pace(1)
	if state := link.Snapshot(); state.OutputA == 0 || state.OutputB != 0 {
		t.Fatalf("explicit test must play only the selected channel: %+v", state)
	}
	fixture.pace(estim.ReferencePackets)
	if status := fixture.owner.Status(); status.ReferencePulse {
		t.Fatal("the reference pulse must end on its own")
	}
	before := link.Snapshot().Packets
	fixture.pace(5)
	if after := link.Snapshot().Packets; after != before {
		t.Fatalf("idle ticks sent %d packets; nothing should play when idle", after-before)
	}
	if _, err := fixture.owner.AdjustStrength(estim.ChannelA, 50); err != nil {
		t.Fatal(err)
	}
	fixture.pace(1)
	fixture.waitFor("ceiling", func(status CoyoteStatus) bool { return status.StrengthA == 20 })
	if _, err := fixture.owner.AdjustStrength(estim.ChannelA, 1); !errors.Is(err, estim.ErrStrengthCeiling) {
		t.Fatalf("raise at the ceiling = %v", err)
	}
}

func TestCoyoteStreamPlaysFadesInAndStops(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	fixture.confirm()
	ctx := context.Background()
	if _, err := fixture.owner.AppendPoints(ctx, strokeCommand("s1", 8000)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.owner.Play(ctx, PlayCommand{StreamID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if fixture.owner.PlaybackStartTime().IsZero() {
		t.Fatal("Play must report its start time")
	}
	fixture.pace(1)
	early := link.Snapshot()
	fixture.pace(30)
	late := link.Snapshot()
	if early.OutputA+early.OutputB >= late.OutputA+late.OutputB || late.OutputA+late.OutputB == 0 {
		t.Fatalf("output must fade in: early %+v late %+v", early, late)
	}
	if late.StrengthA != 0 || late.StrengthB != 0 {
		t.Fatalf("playback must never change strength: %+v", late)
	}
	if diagnostics := fixture.owner.Diagnostics(); diagnostics.PlaybackState != "playing" || !diagnostics.Connected {
		t.Fatalf("diagnostics while playing = %+v", diagnostics)
	}
	if _, err := fixture.owner.Stop(ctx, StopCommand{Reason: "pause"}); err != nil {
		t.Fatal(err)
	}
	stopped := link.Snapshot()
	if stopped.OutputA != 0 || stopped.OutputB != 0 {
		t.Fatalf("Stop must leave the device silent: %+v", stopped)
	}
	fixture.pace(5)
	if after := link.Snapshot().Packets; after != stopped.Packets {
		t.Fatal("packets were sent after Stop")
	}
}

func TestCoyoteReportsStarvationWhenTheFrameRunsOut(t *testing.T) {
	fixture := newCoyoteFixture(t)
	fixture.connect()
	fixture.confirm()
	ctx := context.Background()
	_, _ = fixture.owner.AppendPoints(ctx, strokeCommand("short", 500))
	_, _ = fixture.owner.Play(ctx, PlayCommand{StreamID: "short"})
	fixture.pace(12)
	if state := fixture.owner.Diagnostics().PlaybackState; state != "starving" && state != "starved" {
		t.Fatalf("state after the frame ran out = %q", state)
	}
	fixture.pace(10)
	if state := fixture.owner.Diagnostics().PlaybackState; state != "starved" {
		t.Fatalf("state = %q, want starved", state)
	}
}

func TestCoyoteEmergencyZeroEndsOutputAndZeroesStrength(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	fixture.confirm()
	_, _ = fixture.owner.AdjustStrength(estim.ChannelB, 6)
	fixture.pace(1)
	fixture.waitFor("raise", func(status CoyoteStatus) bool { return status.StrengthB == 6 })
	ctx := context.Background()
	_, _ = fixture.owner.AppendPoints(ctx, strokeCommand("s", 4000))
	_, _ = fixture.owner.Play(ctx, PlayCommand{StreamID: "s"})
	fixture.pace(3)
	// A step queued just before Stop must not survive it.
	_, _ = fixture.owner.AdjustStrength(estim.ChannelB, 2)
	if err := fixture.owner.EmergencyZero(); err != nil {
		t.Fatal(err)
	}
	fixture.waitFor("zero", func(status CoyoteStatus) bool { return status.StrengthB == 0 && status.StrengthKnown })
	fixture.pace(5)
	state := link.Snapshot()
	if state.StrengthA != 0 || state.StrengthB != 0 || state.OutputA != 0 || state.OutputB != 0 {
		t.Fatalf("after EmergencyZero = %+v", state)
	}
}

func TestCoyotePacketComposedBeforeStopNeverFollowsIt(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	fixture.confirm()
	ctx := context.Background()
	_, _ = fixture.owner.AppendPoints(ctx, strokeCommand("s", 4000))
	_, _ = fixture.owner.Play(ctx, PlayCommand{StreamID: "s"})
	fixture.clock.Advance(3 * time.Second)
	fixture.owner.mu.Lock()
	stale, ok := fixture.owner.composeLocked(fixture.clock.Now())
	generation := fixture.owner.generation
	fixture.owner.mu.Unlock()
	if !ok {
		t.Fatal("no packet composed while playing")
	}
	_, _ = fixture.owner.Stop(ctx, StopCommand{Reason: "stop"})
	if err := fixture.owner.write(generation, link, stale, false); !errors.Is(err, errCoyoteSuperseded) {
		t.Fatalf("stale packet write = %v, want errCoyoteSuperseded", err)
	}
}

func TestCoyoteLinkLossIsReportedAndRequiresReconnect(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	ctx := context.Background()
	_, _ = fixture.owner.AppendPoints(ctx, strokeCommand("s", 4000))
	_, _ = fixture.owner.Play(ctx, PlayCommand{StreamID: "s"})
	link.Lose(errors.New("radio dropped"))
	status := fixture.waitFor("release", func(status CoyoteStatus) bool { return !status.Connected })
	if status.PlaybackState != "stale" || status.LastError == "" {
		t.Fatalf("status after loss = %+v", status)
	}
	if diagnostics := fixture.owner.Diagnostics(); diagnostics.PlaybackState != "stale" || diagnostics.Connected {
		t.Fatalf("the engine must see the loss: %+v", diagnostics)
	}
	if _, err := fixture.owner.Play(ctx, PlayCommand{StreamID: "s"}); !errors.Is(err, errCoyoteNotConnected) {
		t.Fatalf("Play after loss = %v", err)
	}
	if _, err := fixture.owner.AdjustStrength(estim.ChannelA, 1); !errors.Is(err, errCoyoteNotConnected) {
		t.Fatalf("strength after loss = %v", err)
	}
}

func TestCoyoteDisconnectZeroesAndCloseRefusesReconnect(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	fixture.confirm()
	_, _ = fixture.owner.AdjustStrength(estim.ChannelA, 4)
	fixture.pace(1)
	fixture.waitFor("raise", func(status CoyoteStatus) bool { return status.StrengthA == 4 })
	if _, err := fixture.owner.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if state := link.Snapshot(); state.StrengthA != 0 || !state.Closed {
		t.Fatalf("Disconnect must zero strength and close the link: %+v", state)
	}
	if err := fixture.owner.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.owner.Connect(context.Background()); !errors.Is(err, errCoyoteClosed) {
		t.Fatalf("Connect after Close = %v", err)
	}
}

func TestCoyoteConfigureRewritesCeilingsLive(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	if err := fixture.owner.Configure(CoyoteConfig{Limits: estim.Limits{StrengthA: 8, StrengthB: 9, FrequencyBalanceA: 160, FrequencyBalanceB: 160}, Profile: estim.Profile{Follow: estim.FollowSpeed, MinHz: 40, MaxHz: 100}}); err != nil {
		t.Fatal(err)
	}
	if state := link.Snapshot(); state.Limits != 2 || state.CeilingA != 8 || state.CeilingB != 9 {
		t.Fatalf("a ceiling change must reach the connected device: %+v", state)
	}
	if status := fixture.owner.Status(); status.CeilingA != 8 || status.CeilingB != 9 {
		t.Fatalf("status ceilings = %+v", status)
	}
}

func TestCoyoteCeilingWriteIsNeverSupersededByStop(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	_, _ = fixture.owner.Stop(context.Background(), StopCommand{Reason: "stop"})
	_ = fixture.owner.EmergencyZero()
	if err := fixture.owner.Configure(CoyoteConfig{Limits: estim.Limits{StrengthA: 5, StrengthB: 5, FrequencyBalanceA: 160, FrequencyBalanceB: 160}, Profile: estim.Profile{Follow: estim.FollowStroke, MinHz: 40, MaxHz: 100}}); err != nil {
		t.Fatal(err)
	}
	if state := link.Snapshot(); state.CeilingA != 5 || state.CeilingB != 5 {
		t.Fatalf("a ceiling change after Stop must still reach the device: %+v", state)
	}
}

func TestCoyoteConnectFailureLeavesTheOwnerDisconnected(t *testing.T) {
	fixture := newCoyoteFixture(t)
	fixture.connector.FailNextConnect(estim.ErrNotFound)
	status, err := fixture.owner.Connect(context.Background())
	if !errors.Is(err, estim.ErrNotFound) || status.Connected || status.Connecting || status.LastError == "" {
		t.Fatalf("failed connect = %+v, %v", status, err)
	}
	if _, err := fixture.owner.AppendPoints(context.Background(), strokeCommand("s", 500)); !errors.Is(err, errCoyoteNotConnected) {
		t.Fatalf("AppendPoints without a device = %v", err)
	}
}

func TestFanOutDrivesBothOwnersAndKeepsTheHandyAuthoritative(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	fixture.confirm()
	handy := NewFake()
	fanOut := NewFanOut(handy, fixture.owner)
	if _, ok := fanOut.(MotionStartupStateProvider); ok {
		t.Fatal("the fake Handy has no startup reader; the fan-out must not claim one")
	}
	ctx := context.Background()
	if _, err := fanOut.AppendPoints(ctx, strokeCommand("both", 6000)); err != nil {
		t.Fatal(err)
	}
	if _, err := fanOut.Play(ctx, PlayCommand{StreamID: "both"}); err != nil {
		t.Fatal(err)
	}
	fixture.pace(30)
	if state := link.Snapshot(); state.OutputA+state.OutputB == 0 {
		t.Fatalf("the Coyote did not follow the shared frame: %+v", state)
	}
	if diagnostics := fanOut.Diagnostics(); diagnostics.Name != fakeTransportName {
		t.Fatalf("fan-out diagnostics = %q, want the Handy's", diagnostics.Name)
	}
	capabilities := fanOut.(MotionTimingCapabilitiesProvider).MotionTimingCapabilities()
	if capabilities.MinimumBufferedLead < coyoteMinimumLead {
		t.Fatalf("lead %v must cover the Coyote's render lead", capabilities.MinimumBufferedLead)
	}
	if _, err := fanOut.Stop(ctx, StopCommand{Reason: "stop"}); err != nil {
		t.Fatal(err)
	}
	commands := handy.Commands()
	if commands[len(commands)-1].Kind != CommandKindStop {
		t.Fatal("Stop must reach the Handy")
	}
	if state := fixture.owner.Diagnostics().PlaybackState; state != "idle" {
		t.Fatalf("Stop must reach the Coyote, state %q", state)
	}
}

func TestFanOutCoyoteLossStopsTheLinkedRun(t *testing.T) {
	fixture := newCoyoteFixture(t)
	link := fixture.connect()
	fixture.confirm()
	handy := NewFake()
	fanOut := NewFanOut(handy, fixture.owner)
	ctx := context.Background()
	_, _ = fanOut.AppendPoints(ctx, strokeCommand("both", 6000))
	_, _ = fanOut.Play(ctx, PlayCommand{StreamID: "both"})
	link.Lose(errors.New("radio dropped"))
	fixture.waitFor("release", func(status CoyoteStatus) bool { return !status.Connected })
	if _, err := fanOut.AppendPoints(ctx, strokeCommand("both", 7000)); err == nil {
		t.Fatal("a linked run must fail when its Coyote drops")
	}
	if diagnostics := fanOut.Diagnostics(); diagnostics.PlaybackState != "stale" {
		t.Fatalf("fan-out must report a failed linked session, got %q", diagnostics.PlaybackState)
	}
}

// A connected Coyote names what the user must raise before anything is felt:
// every connection starts at zero, and a zero ceiling allows nothing (ADR 0063).
func TestCoyoteReportsWhyItIsSilent(t *testing.T) {
	fixture := newCoyoteFixture(t)
	if status := fixture.owner.Status(); status.SilentReason != "" {
		t.Fatalf("disconnected: %q", status.SilentReason)
	}
	fixture.connect()
	fixture.confirm()
	// A commanded zero is not a measurement: silence needs the device's report.
	if status := fixture.owner.Status(); status.SilentReason != "" || status.StrengthSource != "zero_command" {
		t.Fatalf("a new connection = %q (%s), want no claim yet", status.SilentReason, status.StrengthSource)
	}
	if _, err := fixture.owner.AdjustStrength(estim.ChannelB, 2); err != nil {
		t.Fatal(err)
	}
	fixture.pace(1)
	if status := fixture.waitFor("raise report", func(status CoyoteStatus) bool { return status.StrengthB == 2 }); status.SilentReason != "" {
		t.Fatalf("a raised channel = %q", status.SilentReason)
	}
	if _, err := fixture.owner.AdjustStrength(estim.ChannelB, -2); err != nil {
		t.Fatal(err)
	}
	fixture.pace(1)
	if status := fixture.waitFor("lower report", func(status CoyoteStatus) bool { return status.StrengthB == 0 && !status.StrengthPending }); status.SilentReason != "strength_zero" {
		t.Fatalf("a reported zero = %q (%s), want strength_zero", status.SilentReason, status.StrengthSource)
	}
	if err := fixture.owner.Configure(CoyoteConfig{Profile: estim.Profile{Follow: estim.FollowStroke, MinHz: 40, MaxHz: 100}}); err != nil {
		t.Fatal(err)
	}
	fixture.pace(1)
	if status := fixture.waitFor("zero ceilings", func(status CoyoteStatus) bool { return status.LimitsApplied && status.CeilingA == 0 }); status.SilentReason != "ceiling_zero" {
		t.Fatalf("zero ceilings = %q (%+v)", status.SilentReason, status)
	}
}
