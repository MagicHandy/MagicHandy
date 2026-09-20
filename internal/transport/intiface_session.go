package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/coder/websocket"
)

// Connect opens the websocket and completes the Buttplug v3 handshake and device list request.
func (i *Intiface) Connect(ctx context.Context) error {
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()

	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return errors.New("Intiface owner is closed")
	}
	if i.started {
		i.mu.Unlock()
		return errors.New("Intiface connection has already been started")
	}
	i.started = true
	i.mu.Unlock()

	conn, response, err := websocket.Dial(ctx, i.options.Address, &websocket.DialOptions{HTTPClient: i.options.HTTPClient})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		i.setSessionFailure(fmt.Errorf("connect to Intiface: %w", err))
		return err
	}
	conn.SetReadLimit(1 << 20)
	sessionCtx, sessionStop := context.WithCancel(context.Background())
	i.mu.Lock()
	i.conn = conn
	i.sessionCtx = sessionCtx
	i.sessionStop = sessionStop
	i.connected = true
	i.diagnosis.Connected = true
	i.diagnosis.PlaybackState = "idle"
	i.mu.Unlock()

	i.wg.Add(1)
	go i.readLoop(sessionCtx, conn)

	if err := i.handshake(ctx); err != nil {
		i.abortConnect(err)
		return err
	}
	i.mu.Lock()
	connected := i.connected && sessionCtx.Err() == nil
	i.mu.Unlock()
	if !connected {
		err := errors.New("Intiface connection ended during discovery")
		i.abortConnect(err)
		return err
	}

	i.wg.Add(1)
	go i.paceLoop(sessionCtx)
	return nil
}

func (i *Intiface) handshake(ctx context.Context) error {
	message, err := i.request(ctx, "RequestServerInfo", map[string]any{
		"ClientName":     i.options.ClientName,
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
	i.mu.Lock()
	i.maxPingTime = time.Duration(info.MaxPingTime) * time.Millisecond
	sessionCtx := i.sessionCtx
	i.mu.Unlock()
	if info.MaxPingTime > 0 {
		i.wg.Add(1)
		go i.pingLoop(sessionCtx, time.Duration(info.MaxPingTime)*time.Millisecond)
	}

	message, err = i.request(ctx, "RequestDeviceList", nil)
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

// Close stops pacing, attempts a device stop, and tears down all session goroutines.
func (i *Intiface) Close() error {
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()

	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return nil
	}
	i.closed = true
	i.scanning = false
	connected := i.connected
	selected := i.selected
	stopSession := i.sessionStop
	conn := i.conn
	i.mu.Unlock()

	i.paceMu.Lock()
	i.invalidatePlaybackLocked(true)
	i.paceMu.Unlock()
	i.signalPacer()
	i.workerMu.Lock()
	i.workersClosed = true
	i.workerMu.Unlock()

	var stopErr error
	if connected && selected {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_, stopErr = i.Stop(ctx, StopCommand{Reason: "owner_close"})
		cancel()
	}
	if stopSession != nil {
		stopSession()
	}
	var closeErr error
	if conn != nil && connected {
		closeErr = conn.CloseNow()
	}
	i.wg.Wait()
	i.mu.Lock()
	i.connected = false
	i.scanning = false
	i.diagnosis.Connected = false
	i.mu.Unlock()
	if closeErr == nil {
		return stopErr
	}
	return closeErr
}

func (i *Intiface) readLoop(ctx context.Context, conn *websocket.Conn) {
	defer i.wg.Done()
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				i.setSessionFailure(fmt.Errorf("read Intiface websocket: %w", err))
			}
			return
		}
		if messageType != websocket.MessageText {
			i.setSessionFailure(errors.New("Intiface sent a non-text websocket message"))
			return
		}
		messages, err := decodeButtplugMessages(data)
		if err != nil {
			i.setSessionFailure(err)
			return
		}
		for _, message := range messages {
			if message.kind == "Error" && message.id == 0 {
				i.setSessionFailure(errors.New("Intiface reported an uncorrelated protocol error"))
				return
			}
			switch message.kind {
			case "DeviceAdded":
				i.handleDeviceAdded(message.payload)
			case "DeviceRemoved":
				i.handleDeviceRemoved(message.payload)
			case "ScanningFinished":
				i.mu.Lock()
				i.scanning = false
				i.mu.Unlock()
			default:
				i.routeResponse(message)
			}
		}
	}
}

func (i *Intiface) pingLoop(ctx context.Context, maxPingTime time.Duration) {
	defer i.wg.Done()
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
			err := i.requestOK(pingCtx, "Ping", nil)
			cancel()
			if err != nil && ctx.Err() == nil {
				i.setSessionFailure(fmt.Errorf("Intiface ping failed: %w", err))
				return
			}
		}
	}
}

func (i *Intiface) request(ctx context.Context, kind string, fields map[string]any) (buttplugMessage, error) {
	request, err := i.startRequest(ctx, kind, fields, 0)
	if err != nil {
		return buttplugMessage{}, err
	}
	message, err := i.awaitResponse(ctx, request)
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

func (i *Intiface) requestOK(ctx context.Context, kind string, fields map[string]any) error {
	message, err := i.request(ctx, kind, fields)
	if err != nil {
		return err
	}
	if message.kind != "Ok" {
		return fmt.Errorf("expected Buttplug Ok for %s, received %s", kind, message.kind)
	}
	return nil
}

func (i *Intiface) startRequest(ctx context.Context, kind string, fields map[string]any, generation uint64) (intifaceRequest, error) {
	return i.startRequestGuarded(ctx, kind, fields, generation, nil)
}

func (i *Intiface) startPacedRequest(ctx context.Context, fields map[string]any, generation uint64, paced intifacePacedRequest) (intifaceRequest, error) {
	return i.startRequestGuarded(ctx, "LinearCmd", fields, generation, &paced)
}

func (i *Intiface) startRequestGuarded(ctx context.Context, kind string, fields map[string]any, generation uint64, paced *intifacePacedRequest) (intifaceRequest, error) {
	i.mu.Lock()
	if !i.connected || i.conn == nil || (i.closed && kind != "StopDeviceCmd") {
		i.mu.Unlock()
		return intifaceRequest{}, errors.New("Intiface connection is stale")
	}
	i.nextID++
	if i.nextID == 0 {
		i.nextID++
	}
	id := i.nextID
	waiter := &intifaceWaiter{response: make(chan buttplugMessage, 1), kind: kind}
	i.waiters[id] = waiter
	conn := i.conn
	sessionCtx := i.sessionCtx
	i.mu.Unlock()

	i.writeMu.Lock()
	if i.requestClosed(kind) {
		i.writeMu.Unlock()
		i.removeWaiter(id, waiter)
		return intifaceRequest{}, errors.New("Intiface owner is closed")
	}
	admission, err := i.admitRequest(kind, fields, generation, paced)
	if err != nil {
		return i.rejectRequestWrite(id, waiter, admission.linearReserved, err)
	}
	if err := requestContextError(ctx, sessionCtx); err != nil {
		return i.rejectRequestWrite(id, waiter, admission.linearReserved, err)
	}
	data, err := encodeIntifaceRequest(id, kind, fields)
	if err != nil {
		return i.rejectRequestWrite(id, waiter, admission.linearReserved, err)
	}
	// coder/websocket closes the connection when a write context is canceled.
	// Let an admitted frame finish under a short transport-owned deadline so
	// canceling playback cannot tear down the connection needed by Stop.
	writeTimeout, err := intifaceRequestWriteTimeout(admission.latestWrite)
	if err != nil {
		return i.rejectRequestWrite(id, waiter, admission.linearReserved, err)
	}
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), writeTimeout)
	err = conn.Write(writeCtx, websocket.MessageText, data)
	writtenAt := time.Now()
	cancelWrite()
	i.writeMu.Unlock()
	if err != nil {
		i.releasePendingACK(admission.linearReserved)
		i.removeWaiter(id, waiter)
		if sessionCtx.Err() == nil && !errors.Is(err, context.Canceled) {
			i.setSessionFailure(err)
		}
		return intifaceRequest{}, err
	}
	if !admission.latestWrite.IsZero() && writtenAt.After(admission.latestWrite) {
		i.releasePendingACK(admission.linearReserved)
		i.removeWaiter(id, waiter)
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

func (i *Intiface) requestClosed(kind string) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.closed && kind != "StopDeviceCmd"
}

func (i *Intiface) admitRequest(kind string, fields map[string]any, generation uint64, paced *intifacePacedRequest) (intifaceWriteAdmission, error) {
	admission := intifaceWriteAdmission{}
	if generation == 0 {
		return admission, nil
	}
	i.paceMu.Lock()
	defer i.paceMu.Unlock()
	if !i.playing || i.generation != generation {
		return admission, errPacerSuperseded
	}
	if paced != nil {
		var err error
		admission, err = i.preparePacedFieldsLocked(fields, *paced)
		if err != nil {
			return admission, err
		}
	}
	if kind != "LinearCmd" {
		return admission, nil
	}
	if i.pendingACKs >= maxIntifacePendingACKs {
		return admission, errIntifacePendingACKCap
	}
	i.pendingACKs++
	admission.linearReserved = true
	return admission, nil
}

func (i *Intiface) preparePacedFieldsLocked(fields map[string]any, paced intifacePacedRequest) (intifaceWriteAdmission, error) {
	admission := intifaceWriteAdmission{wireDuration: paced.originalDuration}
	var err error
	if paced.enforceSchedule {
		admission.lateness, admission.wireDuration, err = decideIntifaceLiveDuration(
			time.Now(), paced.scheduledAt, paced.scheduledEnd, paced.originalDuration, paced.minimumDuration,
		)
		if err == nil {
			admission.latestWrite, err = latestIntifaceWriteTime(paced)
		}
	}
	vectors, ok := fields["Vectors"].([]map[string]any)
	if !ok || len(vectors) != 1 {
		return admission, &intifacePacerError{category: "invalid_dispatch"}
	}
	if err != nil {
		return admission, err
	}
	vectors[0]["Position"] = projectIntifacePosition(paced.position, i.window)
	if paced.enforceSchedule {
		vectors[0]["Duration"] = admission.wireDuration.Milliseconds()
	}
	return admission, nil
}

func (i *Intiface) rejectRequestWrite(id uint32, waiter *intifaceWaiter, linearReserved bool, err error) (intifaceRequest, error) {
	i.releasePendingACK(linearReserved)
	i.writeMu.Unlock()
	i.removeWaiter(id, waiter)
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

func (i *Intiface) awaitResponse(ctx context.Context, request intifaceRequest) (buttplugMessage, error) {
	timeout := i.responseTimeout
	if timeout <= 0 {
		timeout = defaultIntifaceResponseTime
	}
	remaining := time.Until(request.writtenAt.Add(timeout))
	if remaining <= 0 {
		return i.abandonRequest(request, errIntifaceResponseTime)
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case message := <-request.waiter.response:
		return message, nil
	case <-ctx.Done():
		return i.abandonRequest(request, ctx.Err())
	case <-request.sessionCtx.Done():
		return i.abandonRequest(request, errors.New("Intiface connection is stale"))
	case <-timer.C:
		return i.abandonRequest(request, errIntifaceResponseTime)
	}
}

func (i *Intiface) abandonRequest(request intifaceRequest, err error) (buttplugMessage, error) {
	if i.removeWaiter(request.id, request.waiter) {
		return buttplugMessage{}, err
	}
	// A response removed this waiter first and published to the buffered
	// channel while holding mu. Prefer that response at the deadline boundary.
	return <-request.waiter.response, nil
}

func (i *Intiface) routeResponse(message buttplugMessage) {
	if message.id == 0 {
		return
	}
	i.mu.Lock()
	waiter := i.waiters[message.id]
	if waiter != nil {
		i.applyResponseLocked(waiter.kind, message)
		delete(i.waiters, message.id)
		waiter.response <- message
	}
	i.mu.Unlock()
}

// Apply acknowledged session state on the reader, before later events can
// supersede it. Request goroutines must not replay older snapshots after waking.
func (i *Intiface) applyResponseLocked(requestKind string, message buttplugMessage) {
	if requestKind == "RequestDeviceList" && message.kind == "DeviceList" {
		var list buttplugDeviceList
		if json.Unmarshal(message.payload, &list) == nil {
			i.devices = make(map[uint32]IntifaceDevice, len(list.Devices))
			for _, device := range list.Devices {
				i.devices[device.DeviceIndex] = intifaceDeviceFromProtocol(device)
			}
		}
	}
	if message.kind == "Ok" {
		switch requestKind {
		case "StartScanning":
			i.scanning = true
		case "StopScanning":
			i.scanning = false
		}
	}
}

func (i *Intiface) removeWaiter(id uint32, waiter *intifaceWaiter) bool {
	i.mu.Lock()
	removed := false
	if i.waiters[id] == waiter {
		delete(i.waiters, id)
		removed = true
	}
	i.mu.Unlock()
	return removed
}

func (i *Intiface) setSessionFailure(err error) {
	i.mu.Lock()
	wasConnected := i.connected
	i.connected = false
	i.scanning = false
	i.diagnosis.Connected = false
	i.diagnosis.PlaybackState = "stale"
	i.diagnosis.LastError = err.Error()
	stop := i.sessionStop
	conn := i.conn
	i.mu.Unlock()
	if !wasConnected {
		return
	}
	if stop != nil {
		stop()
	}
	if conn != nil {
		_ = conn.CloseNow()
	}
	i.cancelPlayback("stale")
}

func (i *Intiface) abortConnect(err error) {
	i.setSessionFailure(err)
	i.wg.Wait()
}
