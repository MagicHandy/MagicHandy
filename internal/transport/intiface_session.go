package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// intifaceSessionOwner is an output bound to a session. The session asks it
// to admit its own writes under the writer lock and tells it when the session
// connects, fails, closes or loses one of its devices. Methods ending in
// Locked run with the session lock held: they must not call the session, and
// any follow-up they return runs after the lock is released.
type intifaceSessionOwner interface {
	admitWrite(kind string, fields map[string]any, generation uint64, paced *intifacePacedRequest) (intifaceWriteAdmission, error)
	releaseWrite(reserved bool)
	sessionConnectedLocked()
	sessionReady(sessionCtx context.Context)
	sessionFailedLocked(err error) func()
	deviceRemovedLocked(deviceIndex uint32) func()
	invalidateForEmergency()
	invalidateForClose()
	stopForClose(connected bool) error
	closedLocked()
}

// Session-wide request capacity. Stop and Ping keep a reserve beyond the
// ordinary limit, so a backlog of output writes can never block them.
const (
	maxIntifaceWaiters      = 32
	intifaceReservedWaiters = 8
)

var (
	errIntifaceRequestCapacity = errors.New("Intiface request capacity reached")
	errIntifaceDeviceClaimed   = errors.New("this Intiface device is already used by another output")
	errIntifaceStopping        = errors.New("an Intiface emergency Stop is in progress")
	errIntifaceOwnerClosed     = errors.New("Intiface owner is closed")
)

// intifaceStopKind passes a closed session and the emergency barrier.
func intifaceStopKind(kind string) bool {
	return kind == "StopDeviceCmd" || kind == "StopAllDevices"
}

// intifaceReservedKind may use the reserved request capacity and passes the
// emergency barrier: Stop must always be deliverable, and the keepalive
// keeps the server's own watchdog satisfied while a Stop is in flight.
func intifaceReservedKind(kind string) bool {
	return intifaceStopKind(kind) || kind == "Ping"
}

// intifaceSession owns one Buttplug v3 websocket connection: handshake,
// keepalive, request correlation, the device registry and scanning. Intiface
// Central serves a single client, so every output reached through one server
// binds to the same session. The session never paces motion itself.
type intifaceSession struct {
	options IntifaceOptions

	lifecycleMu   sync.Mutex
	workerMu      sync.Mutex
	workersClosed bool
	mu            sync.Mutex
	writeMu       sync.Mutex
	conn          *websocket.Conn
	sessionCtx    context.Context
	sessionStop   context.CancelFunc
	started       bool
	closed        bool
	connected     bool
	scanning      bool
	maxPingTime   time.Duration
	nextID        uint32
	waiters       map[uint32]*intifaceWaiter
	devices       map[uint32]IntifaceDevice
	// incarnations distinguish a device index that disappears and returns:
	// an owner bound to the old incarnation cannot command the new device.
	incarnations    map[uint32]uint64
	lastIncarnation uint64
	// claims give each device at most one owner, because a device-level Stop
	// from one owner would also stop another owner's output.
	claims          map[uint32]intifaceSessionOwner
	barrier         int
	owners          []intifaceSessionOwner
	wg              sync.WaitGroup
	responseTimeout time.Duration
}

// newIntifaceSession validates options and returns a disconnected session.
func newIntifaceSession(options IntifaceOptions) (*intifaceSession, error) {
	options.Address = strings.TrimSpace(options.Address)
	if options.Address == "" {
		options.Address = defaultIntifaceAddress
	}
	parsed, err := url.Parse(options.Address)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Host == "" {
		return nil, errors.New("Intiface address must be an absolute ws:// or wss:// URL")
	}
	options.ClientName = strings.TrimSpace(options.ClientName)
	if options.ClientName == "" {
		options.ClientName = defaultIntifaceClientName
	}
	if options.QueueCapacity == 0 {
		options.QueueCapacity = defaultIntifaceQueueCapacity
	}
	if options.QueueCapacity < 1 {
		return nil, errors.New("Intiface queue capacity must be positive")
	}
	return &intifaceSession{
		options:         options,
		waiters:         make(map[uint32]*intifaceWaiter),
		devices:         make(map[uint32]IntifaceDevice),
		incarnations:    make(map[uint32]uint64),
		claims:          make(map[uint32]intifaceSessionOwner),
		responseTimeout: defaultIntifaceResponseTime,
	}, nil
}

func (s *intifaceSession) bind(owner intifaceSessionOwner) {
	s.mu.Lock()
	s.owners = append(s.owners, owner)
	s.mu.Unlock()
}

// Connect opens the websocket and completes the Buttplug v3 handshake and
// device list request. Owners start their workers once the session is ready,
// while the connection lifecycle is still held, so no caller can close the
// session first.
func (s *intifaceSession) Connect(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("Intiface owner is closed")
	}
	if s.started {
		s.mu.Unlock()
		return errors.New("Intiface connection has already been started")
	}
	s.started = true
	s.mu.Unlock()

	conn, response, err := websocket.Dial(ctx, s.options.Address, &websocket.DialOptions{HTTPClient: s.options.HTTPClient})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		s.setSessionFailure(fmt.Errorf("connect to Intiface: %w", err))
		return err
	}
	conn.SetReadLimit(1 << 20)
	sessionCtx, sessionStop := context.WithCancel(context.Background())
	s.mu.Lock()
	s.conn = conn
	s.sessionCtx = sessionCtx
	s.sessionStop = sessionStop
	s.connected = true
	for _, owner := range s.owners {
		owner.sessionConnectedLocked()
	}
	s.mu.Unlock()

	s.wg.Add(1)
	go s.readLoop(sessionCtx, conn)

	if err := s.handshake(ctx); err != nil {
		s.abortConnect(err)
		return err
	}
	s.mu.Lock()
	connected := s.connected && sessionCtx.Err() == nil
	s.mu.Unlock()
	if !connected {
		err := errors.New("Intiface connection ended during discovery")
		s.abortConnect(err)
		return err
	}
	s.mu.Lock()
	owners := append([]intifaceSessionOwner(nil), s.owners...)
	s.mu.Unlock()
	for _, owner := range owners {
		owner.sessionReady(sessionCtx)
	}
	return nil
}

func (s *intifaceSession) handshake(ctx context.Context) error {
	message, err := s.request(ctx, "RequestServerInfo", map[string]any{
		"ClientName":     s.options.ClientName,
		"MessageVersion": buttplugMessageVersion,
	})
	if err != nil {
		return err
	}
	if message.kind != "ServerInfo" {
		return fmt.Errorf("expected Buttplug ServerInfo, received %s", message.kind)
	}
	var info buttplugServerInfo
	if err := json.Unmarshal(message.payload, &info); err != nil {
		return err
	}
	if info.MessageVersion != buttplugMessageVersion {
		return fmt.Errorf("Intiface negotiated Buttplug message version %d, want %d", info.MessageVersion, buttplugMessageVersion)
	}
	if info.MaxPingTime < 0 || info.MaxPingTime > maxIntifaceScheduleMillis {
		return errors.New("Intiface MaxPingTime is outside the supported duration range")
	}
	// The server's watchdog starts at identification, not after enumeration.
	// Keep it alive even when RequestDeviceList is slow.
	s.mu.Lock()
	s.maxPingTime = time.Duration(info.MaxPingTime) * time.Millisecond
	sessionCtx := s.sessionCtx
	s.mu.Unlock()
	if info.MaxPingTime > 0 {
		s.wg.Add(1)
		go s.pingLoop(sessionCtx, time.Duration(info.MaxPingTime)*time.Millisecond)
	}

	message, err = s.request(ctx, "RequestDeviceList", nil)
	if err != nil {
		return err
	}
	if message.kind != "DeviceList" {
		return fmt.Errorf("expected Buttplug DeviceList, received %s", message.kind)
	}
	var list buttplugDeviceList
	// The reader applies the list before routing its response, preserving order
	// with hotplug events in this same frame or the next one.
	return json.Unmarshal(message.payload, &list)
}

// Close stops every owner's playback, attempts their device stops, and tears
// down all session goroutines.
func (s *intifaceSession) Close() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.scanning = false
	connected := s.connected
	stopSession := s.sessionStop
	conn := s.conn
	owners := append([]intifaceSessionOwner(nil), s.owners...)
	s.mu.Unlock()

	for _, owner := range owners {
		owner.invalidateForClose()
	}
	s.workerMu.Lock()
	s.workersClosed = true
	s.workerMu.Unlock()

	var stopErr error
	for _, owner := range owners {
		stopErr = errors.Join(stopErr, owner.stopForClose(connected))
	}
	if stopSession != nil {
		stopSession()
	}
	var closeErr error
	if conn != nil && connected {
		closeErr = conn.CloseNow()
	}
	s.wg.Wait()
	s.mu.Lock()
	s.connected = false
	s.scanning = false
	for _, owner := range owners {
		owner.closedLocked()
	}
	s.mu.Unlock()
	if closeErr == nil {
		return stopErr
	}
	return closeErr
}

// Devices returns the current safe discovered-device snapshot.
func (s *intifaceSession) Devices() []IntifaceDevice {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneIntifaceDevicesLocked(s.devices)
}

// StartScanning asks Intiface to begin device discovery.
func (s *intifaceSession) StartScanning(ctx context.Context) error {
	if err := s.requestOK(ctx, "StartScanning", nil); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed || !s.connected {
		s.mu.Unlock()
		return errors.New("Intiface owner closed while scanning started")
	}
	s.mu.Unlock()
	return nil
}

// StopScanning asks Intiface to end device discovery.
func (s *intifaceSession) StopScanning(ctx context.Context) error {
	return s.requestOK(ctx, "StopScanning", nil)
}

func (s *intifaceSession) handleDeviceAdded(payload json.RawMessage) {
	var device buttplugDevice
	if json.Unmarshal(payload, &device) != nil {
		return
	}
	s.mu.Lock()
	s.devices[device.DeviceIndex] = intifaceDeviceFromProtocol(device)
	s.incarnations[device.DeviceIndex] = s.nextIncarnationLocked()
	s.mu.Unlock()
}

func (s *intifaceSession) nextIncarnationLocked() uint64 {
	s.lastIncarnation++
	return s.lastIncarnation
}

// incarnationLocked reports the current incarnation of a present device.
func (s *intifaceSession) incarnationLocked(deviceIndex uint32) (uint64, bool) {
	if _, ok := s.devices[deviceIndex]; !ok {
		return 0, false
	}
	incarnation, ok := s.incarnations[deviceIndex]
	return incarnation, ok
}

// claimLocked gives owner a device. A device claimed by another owner is
// refused; an owner's own earlier claims stay until it releases them.
func (s *intifaceSession) claimLocked(owner intifaceSessionOwner, deviceIndex uint32) error {
	if holder, ok := s.claims[deviceIndex]; ok && holder != owner {
		return errIntifaceDeviceClaimed
	}
	s.claims[deviceIndex] = owner
	return nil
}

// releaseClaimLocked returns a device owner holds; other claims are kept.
func (s *intifaceSession) releaseClaimLocked(owner intifaceSessionOwner, deviceIndex uint32) {
	if s.claims[deviceIndex] == owner {
		delete(s.claims, deviceIndex)
	}
}

func (s *intifaceSession) handleDeviceRemoved(payload json.RawMessage) {
	var removed buttplugDeviceRemoved
	if json.Unmarshal(payload, &removed) != nil {
		return
	}
	s.mu.Lock()
	delete(s.devices, removed.DeviceIndex)
	delete(s.incarnations, removed.DeviceIndex)
	delete(s.claims, removed.DeviceIndex)
	var followUps []func()
	for _, owner := range s.owners {
		if followUp := owner.deviceRemovedLocked(removed.DeviceIndex); followUp != nil {
			followUps = append(followUps, followUp)
		}
	}
	s.mu.Unlock()
	for _, followUp := range followUps {
		followUp()
	}
}

func (s *intifaceSession) readLoop(ctx context.Context, conn *websocket.Conn) {
	defer s.wg.Done()
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				s.setSessionFailure(fmt.Errorf("read Intiface websocket: %w", err))
			}
			return
		}
		if messageType != websocket.MessageText {
			s.setSessionFailure(errors.New("Intiface sent a non-text websocket message"))
			return
		}
		messages, err := decodeButtplugMessages(data)
		if err != nil {
			s.setSessionFailure(err)
			return
		}
		for _, message := range messages {
			if message.kind == "Error" && message.id == 0 {
				s.setSessionFailure(errors.New("Intiface reported an uncorrelated protocol error"))
				return
			}
			switch message.kind {
			case "DeviceAdded":
				s.handleDeviceAdded(message.payload)
			case "DeviceRemoved":
				s.handleDeviceRemoved(message.payload)
			case "ScanningFinished":
				s.mu.Lock()
				s.scanning = false
				s.mu.Unlock()
			default:
				s.routeResponse(message)
			}
		}
	}
}

func (s *intifaceSession) pingLoop(ctx context.Context, maxPingTime time.Duration) {
	defer s.wg.Done()
	interval := maxPingTime / 2
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			timeout := maxPingTime / 3
			if timeout <= 0 {
				timeout = time.Millisecond
			}
			pingCtx, cancel := context.WithTimeout(ctx, timeout)
			err := s.requestOK(pingCtx, "Ping", nil)
			cancel()
			if err != nil && ctx.Err() == nil {
				s.setSessionFailure(fmt.Errorf("Intiface ping failed: %w", err))
				return
			}
		}
	}
}

func (s *intifaceSession) request(ctx context.Context, kind string, fields map[string]any) (buttplugMessage, error) {
	request, err := s.startRequestFor(ctx, nil, kind, fields, 0, nil)
	if err != nil {
		return buttplugMessage{}, err
	}
	message, err := s.awaitResponse(ctx, request)
	if err != nil {
		return buttplugMessage{}, err
	}
	if message.kind == "Error" {
		var protocolError buttplugError
		_ = json.Unmarshal(message.payload, &protocolError)
		return message, fmt.Errorf("Intiface rejected %s: %s", kind, protocolError.ErrorMessage)
	}
	return message, nil
}

func (s *intifaceSession) requestOK(ctx context.Context, kind string, fields map[string]any) error {
	message, err := s.request(ctx, kind, fields)
	if err != nil {
		return err
	}
	if message.kind != "Ok" {
		return fmt.Errorf("expected Buttplug Ok for %s, received %s", kind, message.kind)
	}
	return nil
}

// startRequestFor writes one request. A request with a playback generation
// belongs to owner, which admits it under the writer lock so a superseded
// generation can never reach the wire.
func (s *intifaceSession) startRequestFor(ctx context.Context, owner intifaceSessionOwner, kind string, fields map[string]any, generation uint64, paced *intifacePacedRequest) (intifaceRequest, error) {
	s.mu.Lock()
	if !s.connected || s.conn == nil || (s.closed && !intifaceStopKind(kind)) {
		s.mu.Unlock()
		return intifaceRequest{}, errors.New("Intiface connection is stale")
	}
	limit := maxIntifaceWaiters
	if intifaceReservedKind(kind) {
		limit += intifaceReservedWaiters
	}
	if len(s.waiters) >= limit {
		s.mu.Unlock()
		return intifaceRequest{}, errIntifaceRequestCapacity
	}
	s.nextID++
	if s.nextID == 0 {
		s.nextID++
	}
	id := s.nextID
	waiter := &intifaceWaiter{response: make(chan buttplugMessage, 1), kind: kind}
	s.waiters[id] = waiter
	conn := s.conn
	sessionCtx := s.sessionCtx
	s.mu.Unlock()

	s.writeMu.Lock()
	if err := s.admissionError(kind); err != nil {
		s.writeMu.Unlock()
		s.removeWaiter(id, waiter)
		return intifaceRequest{}, err
	}
	var admission intifaceWriteAdmission
	if owner != nil && generation != 0 {
		var err error
		if admission, err = owner.admitWrite(kind, fields, generation, paced); err != nil {
			return s.rejectRequestWrite(owner, id, waiter, admission.linearReserved, err)
		}
	}
	if err := requestContextError(ctx, sessionCtx); err != nil {
		return s.rejectRequestWrite(owner, id, waiter, admission.linearReserved, err)
	}
	data, err := encodeIntifaceRequest(id, kind, fields)
	if err != nil {
		return s.rejectRequestWrite(owner, id, waiter, admission.linearReserved, err)
	}
	// coder/websocket closes the connection when a write context is canceled.
	// Let an admitted frame finish under a short transport-owned deadline so
	// canceling playback cannot tear down the connection needed by Stop.
	writeTimeout, err := intifaceRequestWriteTimeout(admission.latestWrite)
	if err != nil {
		return s.rejectRequestWrite(owner, id, waiter, admission.linearReserved, err)
	}
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), writeTimeout)
	err = conn.Write(writeCtx, websocket.MessageText, data)
	writtenAt := time.Now()
	cancelWrite()
	s.writeMu.Unlock()
	if err != nil {
		releaseIntifaceWrite(owner, admission.linearReserved)
		s.removeWaiter(id, waiter)
		if sessionCtx.Err() == nil && !errors.Is(err, context.Canceled) {
			s.setSessionFailure(err)
		}
		return intifaceRequest{}, err
	}
	if !admission.latestWrite.IsZero() && writtenAt.After(admission.latestWrite) {
		releaseIntifaceWrite(owner, admission.linearReserved)
		s.removeWaiter(id, waiter)
		return intifaceRequest{
			id: id, writtenAt: writtenAt, lateness: writtenAt.Sub(paced.scheduledAt),
			wireDuration: admission.wireDuration,
		}, &intifacePacerError{category: "write_late"}
	}
	return intifaceRequest{
		id:           id,
		waiter:       waiter,
		sessionCtx:   sessionCtx,
		writtenAt:    writtenAt,
		linear:       admission.linearReserved,
		lateness:     admission.lateness,
		wireDuration: admission.wireDuration,
	}, nil
}

func releaseIntifaceWrite(owner intifaceSessionOwner, reserved bool) {
	if owner != nil {
		owner.releaseWrite(reserved)
	}
}

// admissionError is checked under the writer lock, so a write admitted
// before the emergency barrier either reaches the wire before StopAllDevices
// or is refused.
func (s *intifaceSession) admissionError(kind string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case intifaceStopKind(kind):
		return nil
	case s.closed:
		return errIntifaceOwnerClosed
	case s.barrier > 0 && !intifaceReservedKind(kind):
		return errIntifaceStopping
	default:
		return nil
	}
}

// EmergencyStop closes admission, invalidates every owner's playback and sends
// StopAllDevices, which stops every device the server has, including ones no
// owner selected. Admission stays closed until the Stop completes, so no
// owner write can follow it. A Stop that cannot be confirmed closes the
// session: the server then stops its devices because the client is gone.
func (s *intifaceSession) EmergencyStop(ctx context.Context) error {
	s.mu.Lock()
	if !s.connected {
		s.mu.Unlock()
		return errors.New("Intiface connection is stale")
	}
	s.barrier++
	owners := append([]intifaceSessionOwner(nil), s.owners...)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.barrier--
		s.mu.Unlock()
	}()
	for _, owner := range owners {
		owner.invalidateForEmergency()
	}
	if err := s.requestOK(ctx, "StopAllDevices", nil); err != nil {
		s.setSessionFailure(fmt.Errorf("Intiface StopAllDevices could not be confirmed: %w", err))
		return err
	}
	return nil
}

func (s *intifaceSession) rejectRequestWrite(owner intifaceSessionOwner, id uint32, waiter *intifaceWaiter, reserved bool, err error) (intifaceRequest, error) {
	releaseIntifaceWrite(owner, reserved)
	s.writeMu.Unlock()
	s.removeWaiter(id, waiter)
	return intifaceRequest{}, err
}

func requestContextError(ctx, sessionCtx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sessionCtx.Err() != nil {
		return errors.New("Intiface connection is stale")
	}
	return nil
}

func encodeIntifaceRequest(id uint32, kind string, fields map[string]any) ([]byte, error) {
	body := make(map[string]any, len(fields)+1)
	body["Id"] = id
	for key, value := range fields {
		body[key] = value
	}
	return json.Marshal([]map[string]any{{kind: body}})
}

func intifaceRequestWriteTimeout(latestWrite time.Time) (time.Duration, error) {
	if latestWrite.IsZero() {
		return intifaceWriteTimeout, nil
	}
	remaining := time.Until(latestWrite)
	if remaining <= 0 {
		return 0, &intifacePacerError{category: "write_late"}
	}
	if remaining < intifaceWriteTimeout {
		return remaining, nil
	}
	return intifaceWriteTimeout, nil
}

func latestIntifaceWriteTime(paced intifacePacedRequest) (time.Time, error) {
	allowedLateness := paced.originalDuration / 4
	if allowedLateness > intifaceLateTolerance {
		allowedLateness = intifaceLateTolerance
	}
	latest := paced.scheduledAt.Add(allowedLateness)
	if paced.minimumDuration > 0 {
		minimumDeadline := paced.scheduledEnd.Add(-paced.minimumDuration)
		if minimumDeadline.Before(latest) {
			latest = minimumDeadline
		}
	}
	if latest.Before(paced.scheduledAt) {
		return time.Time{}, &intifacePacerError{category: "timing_gap"}
	}
	return latest, nil
}

func (s *intifaceSession) awaitResponse(ctx context.Context, request intifaceRequest) (buttplugMessage, error) {
	timeout := s.responseTimeout
	if timeout <= 0 {
		timeout = defaultIntifaceResponseTime
	}
	remaining := time.Until(request.writtenAt.Add(timeout))
	if remaining <= 0 {
		return s.abandonRequest(request, errIntifaceResponseTime)
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case message := <-request.waiter.response:
		return message, nil
	case <-ctx.Done():
		return s.abandonRequest(request, ctx.Err())
	case <-request.sessionCtx.Done():
		return s.abandonRequest(request, errors.New("Intiface connection is stale"))
	case <-timer.C:
		return s.abandonRequest(request, errIntifaceResponseTime)
	}
}

func (s *intifaceSession) abandonRequest(request intifaceRequest, err error) (buttplugMessage, error) {
	if s.removeWaiter(request.id, request.waiter) {
		return buttplugMessage{}, err
	}
	// A response removed this waiter first and published to the buffered
	// channel while holding mu. Prefer that response at the deadline boundary.
	return <-request.waiter.response, nil
}

func (s *intifaceSession) routeResponse(message buttplugMessage) {
	if message.id == 0 {
		return
	}
	s.mu.Lock()
	waiter := s.waiters[message.id]
	if waiter != nil {
		s.applyResponseLocked(waiter.kind, message)
		delete(s.waiters, message.id)
		waiter.response <- message
	}
	s.mu.Unlock()
}

// Apply acknowledged session state on the reader, before later events can
// supersede it. Request goroutines must not replay older snapshots after waking.
func (s *intifaceSession) applyResponseLocked(requestKind string, message buttplugMessage) {
	if requestKind == "RequestDeviceList" && message.kind == "DeviceList" {
		var list buttplugDeviceList
		if json.Unmarshal(message.payload, &list) == nil {
			s.devices = make(map[uint32]IntifaceDevice, len(list.Devices))
			s.incarnations = make(map[uint32]uint64, len(list.Devices))
			for _, device := range list.Devices {
				s.devices[device.DeviceIndex] = intifaceDeviceFromProtocol(device)
				s.incarnations[device.DeviceIndex] = s.nextIncarnationLocked()
			}
			for deviceIndex := range s.claims {
				if _, ok := s.devices[deviceIndex]; !ok {
					delete(s.claims, deviceIndex)
				}
			}
		}
	}
	if message.kind == "Ok" {
		switch requestKind {
		case "StartScanning":
			s.scanning = true
		case "StopScanning":
			s.scanning = false
		}
	}
}

func (s *intifaceSession) removeWaiter(id uint32, waiter *intifaceWaiter) bool {
	s.mu.Lock()
	removed := false
	if s.waiters[id] == waiter {
		delete(s.waiters, id)
		removed = true
	}
	s.mu.Unlock()
	return removed
}

// setSessionFailure ends the connection. Every owner records the failure;
// playback is cancelled only when a live connection was lost.
func (s *intifaceSession) setSessionFailure(err error) {
	s.mu.Lock()
	wasConnected := s.connected
	s.connected = false
	s.scanning = false
	var followUps []func()
	for _, owner := range s.owners {
		if followUp := owner.sessionFailedLocked(err); followUp != nil {
			followUps = append(followUps, followUp)
		}
	}
	stop := s.sessionStop
	conn := s.conn
	s.mu.Unlock()
	if !wasConnected {
		return
	}
	if stop != nil {
		stop()
	}
	if conn != nil {
		_ = conn.CloseNow()
	}
	for _, followUp := range followUps {
		followUp()
	}
}

func (s *intifaceSession) abortConnect(err error) {
	s.setSessionFailure(err)
	s.wg.Wait()
}

func cloneIntifaceDevicesLocked(devices map[uint32]IntifaceDevice) []IntifaceDevice {
	clones := make([]IntifaceDevice, 0, len(devices))
	for _, device := range devices {
		clone := device
		clone.LinearActuators = append([]IntifaceLinearActuator{}, device.LinearActuators...)
		clone.ScalarActuators = append([]IntifaceScalarActuator{}, device.ScalarActuators...)
		clones = append(clones, clone)
	}
	sort.Slice(clones, func(a, b int) bool { return clones[a].DeviceIndex < clones[b].DeviceIndex })
	return clones
}
