package transport

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

// Device IDs of Autoblow's cloud devices, as playback participants know them.
const (
	AutoblowUltraDevice = "autoblow"
	VacuGlideDevice     = "vacuglide"
)

const (
	autoblowLookupTimeout  = 10 * time.Second
	autoblowCommandTimeout = 3 * time.Second
	autoblowStopTimeout    = 5 * time.Second
	autoblowHealthInterval = 5 * time.Second
	// autoblowStopGrace lets a request already on the wire finish, so Stop
	// lands after it. Past it, the request is canceled and Stop is sent twice.
	autoblowStopGrace   = 300 * time.Millisecond
	autoblowStopConfirm = 750 * time.Millisecond
	autoblowEventsIdle  = 75 * time.Second
	autoblowMaxBackoff  = 30 * time.Second
	// autoblowFailureLimit consecutive failed commands end a playing stream.
	autoblowFailureLimit = 3
	autoblowTestDuration = 3 * time.Second
	autoblowMaxOffset    = 500
	autoblowRoundTrips   = 3
)

var (
	errAutoblowNotConnected = errors.New("connect the device first")
	errAutoblowConnected    = errors.New("the device is already connected")
	errAutoblowClosed       = errors.New("the Autoblow owner is closed")
	errAutoblowSuperseded   = errors.New("the command was superseded by Stop")
	errAutoblowSessionEnded = errors.New("the device connection changed; start a new playback session")
	errAutoblowBusy         = errors.New("pause playback before testing the device")
	errAutoblowStale        = errors.New("the device connection was lost; Stop could not be confirmed")
)

// AutoblowOptions configures an Autoblow owner.
type AutoblowOptions struct {
	Client autoblow.Options
	// Simulated names the transport as simulated in diagnostics.
	Simulated bool
	Clock     func() time.Time
	// Tick overrides the pacer interval in tests.
	Tick time.Duration
	// HealthInterval overrides how often the device's state is read.
	HealthInterval time.Duration
	// OnButton receives the device's button presses while it is connected.
	// It runs on the event goroutine and must return quickly.
	OnButton func(device, button string)
}

// AutoblowConfig is what the user saved for one device.
type AutoblowConfig struct {
	Token        string
	OffsetMillis int
	// SpeedScale and StrokeFollow apply to the VacuGlide only.
	SpeedScale   float64
	StrokeFollow bool
}

// AutoblowStatus is a safe snapshot. Commanded values are what MagicHandy
// last sent, not measurements: neither device reports its position.
type AutoblowStatus struct {
	Device             string   `json:"device"`
	Connected          bool     `json:"connected"`
	Connecting         bool     `json:"connecting"`
	Simulated          bool     `json:"simulated"`
	TokenSet           bool     `json:"token_set"`
	Cluster            string   `json:"cluster,omitempty"`
	FirmwareVersion    float64  `json:"firmware_version,omitempty"`
	FirmwareStatus     string   `json:"firmware_status,omitempty"`
	OperationalMode    string   `json:"operational_mode,omitempty"`
	PlaybackState      string   `json:"playback_state"`
	RoundTripMillis    int64    `json:"round_trip_ms"`
	CommandedPercent   *float64 `json:"commanded_percent,omitempty"`
	Testing            bool     `json:"testing"`
	ButtonEvents       bool     `json:"button_events"`
	LastButton         string   `json:"last_button,omitempty"`
	RequestsSent       uint64   `json:"requests_sent"`
	RequestFailures    uint64   `json:"request_failures"`
	RateLimited        uint64   `json:"rate_limited"`
	RateLimitRemaining *int     `json:"rate_limit_remaining,omitempty"`
	LateSegments       uint64   `json:"late_segments,omitempty"`
	StrokeSteps        int      `json:"stroke_steps"`
	StrokeFollow       bool     `json:"stroke_follow,omitempty"`
	ValveOpen          string   `json:"valve_open,omitempty"`
	LastError          string   `json:"last_error,omitempty"`
}

type autoblowLoops struct {
	stop chan struct{}
	done sync.WaitGroup
}

// autoblowSend is one device request on the lane.
type autoblowSend func(ctx context.Context, client *autoblow.Client) error

// autoblowBase is what both Autoblow owners share: the token and the cloud
// connection, one ordered request lane, the button stream, health checks
// and diagnostics. What a command means for motion lives in the device files.
type autoblowBase struct {
	family  string
	device  string
	options AutoblowOptions
	clock   func() time.Time

	opMu   sync.Mutex // connect, disconnect and close
	laneMu sync.Mutex // one device request at a time, in order

	mu             sync.Mutex
	closed         bool
	connecting     bool
	config         AutoblowConfig
	client         *autoblow.Client
	halt           autoblowSend
	info           autoblow.Info
	cluster        string
	loops          *autoblowLoops
	sessionID      uint64
	generation     uint64
	stopHolds      int
	inFlight       context.CancelFunc
	state          string
	mode           string
	lastError      string
	lastButton     string
	buttons        bool
	roundTrip      time.Duration
	failures       int
	healthFailures int
	testing        bool
	requests       uint64
	failed         uint64
	rateLimited    uint64
	diag           autoblowDiagnostics
}

func newAutoblowBase(family, device string, options AutoblowOptions) autoblowBase {
	if options.Clock == nil {
		options.Clock = time.Now
	}
	return autoblowBase{family: family, device: device, options: options, clock: options.Clock, state: "disconnected"}
}

func (b *autoblowBase) transportName() string {
	if b.options.Simulated {
		return b.device + "_simulated"
	}
	return b.device + "_cloud"
}

// SessionID changes only at connection boundaries.
func (b *autoblowBase) SessionID() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessionID
}

// ControlGeneration fences a test or valve step against a later Stop.
func (b *autoblowBase) ControlGeneration() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.generation
}

// Connected reports whether a cloud connection is open.
func (b *autoblowBase) Connected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.client != nil
}

func (b *autoblowBase) configureBase(config AutoblowConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	config.Token = strings.TrimSpace(config.Token)
	config.OffsetMillis = max(-autoblowMaxOffset, min(autoblowMaxOffset, config.OffsetMillis))
	b.config = config
}

// connect looks the device up, reads its firmware, measures the round trip
// and stops it, so every connection starts at rest. start launches the
// device's pacer.
func (b *autoblowBase) connect(ctx context.Context, settle autoblowSend, start func(*autoblowLoops)) error {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	b.mu.Lock()
	switch {
	case b.closed:
		b.mu.Unlock()
		return errAutoblowClosed
	case b.client != nil:
		b.mu.Unlock()
		return errAutoblowConnected
	}
	token := b.config.Token
	b.connecting = true
	b.mu.Unlock()
	client, info, cluster, roundTrip, err := b.open(ctx, token, settle)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connecting = false
	if err != nil {
		b.lastError = err.Error()
		return err
	}
	b.client, b.info, b.cluster, b.roundTrip = client, info, cluster, roundTrip
	b.halt = settle
	b.healthFailures = 0
	b.sessionID++
	b.generation++
	b.state, b.lastError, b.failures, b.mode = "idle", "", 0, ""
	loops := &autoblowLoops{stop: make(chan struct{})}
	b.loops = loops
	loops.done.Add(2)
	go b.events(client, loops)
	go b.health(loops)
	start(loops)
	return nil
}

func (b *autoblowBase) open(ctx context.Context, token string, settle autoblowSend) (*autoblow.Client, autoblow.Info, string, time.Duration, error) {
	client, err := autoblow.New(b.family, token, b.options.Client)
	if err != nil {
		return nil, autoblow.Info{}, "", 0, err
	}
	lookupCtx, cancel := context.WithTimeout(ctx, autoblowLookupTimeout)
	defer cancel()
	connection, err := client.Lookup(lookupCtx)
	if err != nil {
		return nil, autoblow.Info{}, "", 0, err
	}
	info, err := client.Info(lookupCtx)
	if err != nil {
		return nil, autoblow.Info{}, "", 0, err
	}
	// The median of a few state reads estimates how early commands must leave.
	trips := make([]time.Duration, 0, autoblowRoundTrips)
	for range autoblowRoundTrips {
		started := b.clock()
		if _, err := readMode(lookupCtx, b.family, client); err != nil {
			return nil, autoblow.Info{}, "", 0, err
		}
		trips = append(trips, b.clock().Sub(started))
	}
	sort.Slice(trips, func(i, j int) bool { return trips[i] < trips[j] })
	// A new connection always starts at rest, whatever the device was doing.
	if err := settle(lookupCtx, client); err != nil {
		return nil, autoblow.Info{}, "", 0, err
	}
	return client, info, connection.Cluster, trips[len(trips)/2], nil
}

// disconnect stops the device, then closes the connection.
func (b *autoblowBase) disconnect(stop autoblowSend) error {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	return b.disconnectLocked(stop)
}

func (b *autoblowBase) disconnectLocked(stop autoblowSend) error {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()
	if client == nil {
		return nil
	}
	err := b.stopDevice(stop)
	b.release(client, "")
	return err
}

// close disconnects and refuses later connections.
func (b *autoblowBase) close(stop autoblowSend) error {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return b.disconnectLocked(stop)
}

// release ends a connection and waits for its goroutines. It must not run on
// one of them: loss handling calls it with go.
func (b *autoblowBase) release(client *autoblow.Client, reason string) {
	b.mu.Lock()
	if b.client != client || client == nil {
		b.mu.Unlock()
		return
	}
	b.client = nil
	b.sessionID++
	b.generation++
	b.buttons, b.testing, b.mode = false, false, ""
	b.state = "disconnected"
	if reason != "" {
		b.lastError = reason
		b.state = "stale"
	}
	loops := b.loops
	b.loops = nil
	cancel := b.inFlight
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if loops != nil {
		close(loops.stop)
		loops.done.Wait()
	}
}

// events reads the button stream while the connection lasts, reconnecting
// with backoff. A stream with no event or heartbeat for autoblowEventsIdle
// is treated as dead.
func (b *autoblowBase) events(client *autoblow.Client, loops *autoblowLoops) {
	defer loops.done.Done()
	backoff := 2 * time.Second
	for {
		healthy := b.readEvents(client, loops)
		if healthy {
			backoff = 2 * time.Second
		}
		timer := time.NewTimer(backoff)
		select {
		case <-loops.stop:
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff = min(backoff*2, autoblowMaxBackoff)
	}
}

func (b *autoblowBase) readEvents(client *autoblow.Client, loops *autoblowLoops) bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-loops.stop:
			cancel()
		case <-stopped:
		}
	}()
	stream, err := client.Events(ctx)
	if err != nil {
		return false
	}
	defer func() { _ = stream.Close() }()
	idle := time.AfterFunc(autoblowEventsIdle, cancel)
	defer idle.Stop()
	b.setButtons(client, true)
	defer b.setButtons(client, false)
	received := false
	for {
		event, err := stream.Next()
		if err != nil {
			return received
		}
		received = true
		idle.Reset(autoblowEventsIdle)
		if !autoblowButtonKnown(b.family, event.Name) {
			continue
		}
		b.mu.Lock()
		current := b.client == client
		if current {
			b.lastButton = event.Name
		}
		b.mu.Unlock()
		if current && b.options.OnButton != nil {
			b.options.OnButton(b.device, event.Name)
		}
	}
}

func (b *autoblowBase) setButtons(client *autoblow.Client, connected bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.client == client {
		b.buttons = connected
	}
}

// health reads the device's state, playing or not: a steady speed sends no
// commands, so without it a device that left online mode would show as lost
// only at its next change.
func (b *autoblowBase) health(loops *autoblowLoops) {
	defer loops.done.Done()
	interval := b.options.HealthInterval
	if interval <= 0 {
		interval = autoblowHealthInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-loops.stop:
			return
		case <-ticker.C:
		}
		b.mu.Lock()
		busy := b.stopHolds > 0 || b.state == "stale"
		b.mu.Unlock()
		if busy {
			continue
		}
		var checked *autoblow.Client
		err := b.command(0, autoblowCommandTimeout, func(ctx context.Context, client *autoblow.Client) error {
			checked = client
			mode, err := readMode(ctx, b.family, client)
			if err == nil {
				b.mu.Lock()
				b.mode = mode
				b.mu.Unlock()
			}
			return err
		})
		b.mu.Lock()
		if checked != nil && b.client == checked {
			if err == nil {
				b.healthFailures = 0
			} else {
				b.healthFailures++
			}
			if b.healthFailures >= autoblowFailureLimit {
				b.state = "stale"
				b.lastError = "the device stopped answering health checks; Stop could not be confirmed"
				go b.failConnection(checked, errors.New(b.lastError))
			}
		}
		b.mu.Unlock()
	}
}

func readMode(ctx context.Context, family string, client *autoblow.Client) (string, error) {
	if family == autoblow.FamilyVacuGlide {
		state, err := client.VacuGlideState(ctx)
		return state.OperationalMode, errors.Join(err, autoblowModeError(state.OperationalMode))
	}
	state, err := client.UltraState(ctx)
	return state.OperationalMode, errors.Join(err, autoblowModeError(state.OperationalMode))
}

func autoblowModeError(mode string) error {
	switch mode {
	case "ONLINE_CONNECTED", "GO_TO", "OSCILLATOR_PLAYING", "OSCILLATOR_PAUSED", "TARGET_SPEED_PLAYING", "TARGET_SPEED_PAUSED", "LOCAL_SCRIPT_PLAYING", "LOCAL_SCRIPT_PAUSED", "SYNC_SCRIPT_PLAYING", "SYNC_SCRIPT_PAUSED":
		return nil
	default:
		return errors.New("the device is not ready for playback; check its status in Autoblow's app")
	}
}

func autoblowButtonKnown(family, name string) bool {
	if family == autoblow.FamilyUltra {
		return name == autoblow.ButtonUltraPause || name == autoblow.ButtonUltraMode || name == autoblow.ButtonUltraSpeedUp || name == autoblow.ButtonUltraSpeedDown
	}
	return name == autoblow.ButtonVacuGlideMode || name == autoblow.ButtonVacuGlideSpeedPlus || name == autoblow.ButtonVacuGlideSpeedMinus
}

// Run outside a connection's own goroutines: retirement joins them and
// serializes with reconnect/close. Reconnection never revives its old frame.
func (b *autoblowBase) failConnection(client *autoblow.Client, cause error) {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	b.mu.Lock()
	if b.client != client || client == nil {
		b.mu.Unlock()
		return
	}
	b.state = "stale"
	stop := b.halt
	b.mu.Unlock()
	if stop != nil {
		cause = errors.Join(cause, b.stopDevice(stop))
	}
	b.release(client, cause.Error())
}

// noteFailureLocked counts a failed motion command and reports whether the
// stream should end.
func (b *autoblowBase) noteFailureLocked(err error) bool {
	if errors.Is(err, errAutoblowSuperseded) || errors.Is(err, errAutoblowNotConnected) {
		return false
	}
	b.failures++
	b.lastError = err.Error()
	return b.failures >= autoblowFailureLimit || lostConnection(err)
}

// statusLocked fills the fields both devices share.
func (b *autoblowBase) statusLocked() AutoblowStatus {
	status := AutoblowStatus{
		Device:          b.device,
		Connected:       b.client != nil,
		Connecting:      b.connecting,
		Simulated:       b.options.Simulated,
		TokenSet:        b.config.Token != "",
		PlaybackState:   b.state,
		Testing:         b.testing,
		ButtonEvents:    b.buttons,
		LastButton:      b.lastButton,
		RequestsSent:    b.requests,
		RequestFailures: b.failed,
		RateLimited:     b.rateLimited,
		LastError:       b.lastError,
	}
	if b.client != nil {
		status.Cluster = b.cluster
		status.FirmwareVersion = b.info.FirmwareVersion
		status.FirmwareStatus = b.info.FirmwareStatus
		status.OperationalMode = b.mode
		status.RoundTripMillis = b.roundTrip.Milliseconds()
		if limit := b.client.RateLimit(); limit.Known {
			remaining := limit.Remaining
			status.RateLimitRemaining = &remaining
		}
	}
	if b.stopHolds > 0 {
		status.PlaybackState = "stopping"
	}
	return status
}

// leadLocked is how early a command leaves so it acts on time: half the
// measured round trip, plus the user's offset.
func (b *autoblowBase) leadLocked() time.Duration {
	return b.roundTrip/2 + time.Duration(b.config.OffsetMillis)*time.Millisecond
}

type autoblowSessionKey struct{}

// checkSessionLocked rejects a command from an engine bound to an earlier
// connection.
func (b *autoblowBase) checkSessionLocked(ctx context.Context) error {
	if client, ok := ctx.Value(autoblowSessionKey{}).(*autoblow.Client); ok && client != b.client {
		return errAutoblowSessionEnded
	}
	return nil
}

func (b *autoblowBase) recordLocked(command Command, err error) (CommandResult, error) {
	return b.diag.record(command, b.transportName(), b.clock(), err), err
}

func (b *autoblowBase) diagnosticsLocked() TransportDiagnostics {
	diagnostics := b.diag.snapshot()
	diagnostics.Name = b.transportName()
	diagnostics.Connected = b.client != nil
	diagnostics.PlaybackState = b.state
	if b.stopHolds > 0 {
		diagnostics.PlaybackState = "stopping"
	}
	diagnostics.LastLatencyMillis = b.roundTrip.Milliseconds()
	diagnostics.LastError = redactedError(b.lastError)
	return diagnostics
}

// autoblowDiagnostics records the command history the engine reads.
type autoblowDiagnostics struct {
	nextID      int
	count       int
	lastCommand *Command
	lastResult  *CommandResult
}

func (d *autoblowDiagnostics) record(command Command, name string, now time.Time, err error) CommandResult {
	d.nextID++
	d.count++
	command.ID = fmt.Sprintf("%s-%06d", name, d.nextID)
	command.IssuedAt = now.UTC().Format(time.RFC3339Nano)
	result := CommandResult{CommandID: command.ID, Kind: command.Kind, Transport: name, OK: err == nil, Status: "accepted", CompletedAt: command.IssuedAt}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	safe := SafeCommand(command)
	safeResult := SafeCommandResult(result)
	d.lastCommand, d.lastResult = &safe, &safeResult
	return result
}

func (d *autoblowDiagnostics) snapshot() TransportDiagnostics {
	diagnostics := TransportDiagnostics{CommandCount: d.count}
	if d.lastCommand != nil {
		command := cloneCommand(*d.lastCommand)
		diagnostics.LastCommand = &command
	}
	if d.lastResult != nil {
		result := *d.lastResult
		diagnostics.LastResult = &result
	}
	return diagnostics
}
