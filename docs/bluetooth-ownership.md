# Bluetooth Ownership Decision

## Status

Initial decision: browser-owned Bluetooth bridge for early MagicHandy implementation.

Update 2026-10-14: [ADR 0046](decisions/0046-shared-native-bluetooth-worker.md)
adds an independent host-owned Handy option. Browser Bluetooth remains supported.

## Context

StrokeGPT-ReVibed added experimental Web Bluetooth support where the browser owns the BLE connection and the server bridges motion commands to the active browser. This was chosen because browser Web Bluetooth is practical for a local web app, while native server-side Bluetooth on Windows can be difficult and inconsistent.

MagicHandy needs a clear ownership model before implementing Bluetooth.

## Options

### Option A: Browser-Owned Web Bluetooth

The browser owns the BLE connection. The Go server owns motion planning, semantic state, and transport semantics. The browser exposes a bridge protocol that accepts encoded Handy commands from the server and returns status/results.

Pros:

- aligns with current practical approach
- avoids native Windows BLE complexity in the Go core
- keeps permission prompts in the browser where Web Bluetooth expects them
- easier to hide/show Bluetooth UI based on settings

Cons:

- requires an active browser tab
- server cannot control BLE if the tab is closed
- bridge protocol must be robust and observable
- browser compatibility constraints apply

### Option B: Native Go Bluetooth

The Go app owns the BLE connection directly.

Pros:

- does not require browser tab ownership
- could support a more native app model later
- transport state lives fully in the Go core

Cons:

- Windows BLE support in Go may be painful
- driver/device pairing issues may be harder to diagnose
- adds platform-specific complexity early
- risks delaying the core rewrite for a non-core transport path

## Decision

Use browser-owned Web Bluetooth for the initial MagicHandy Bluetooth implementation.

The Go core should model Bluetooth as a second HSP dispatch owner with the same high-level semantics as Cloud REST. It is not a separate motion backend or fallback transport. The difference is dispatch ownership:

- Go owns motion planning, state, traces, and command intent.
- Browser owns BLE permission, connection, protobuf encoding if needed, dispatch, and immediate BLE result.
- The bridge reports status back to Go.

## Required Bridge Contract

The bridge must expose:

- connection status
- device name/id if safe
- firmware/protocol capability if available
- command request ID
- command result/error
- disconnect events
- stale-tab detection
- explicit no-cloud-fallback behavior when the Bluetooth dispatch owner is selected

Hardware validation on 2026-07-02 showed that some Handy BLE operations are
write-only in practice. The Browser Bluetooth connection check therefore
reports bridge readiness and HSP availability from the active browser heartbeat
instead of probing `hsp/state`; explicit state reads remain diagnostics. The
`hsp/play` bridge path is also treated as write-ack on successful BLE write, so
the motion engine does not wait for a response that the device may not emit.

Semantic stroke-window values crossing the bridge remain percentages in the
inclusive 0-100 range. The browser encoder clamps that semantic range and divides by 100 exactly
once for the normalized `RequestSliderStrokeSet` floats. It never guesses
units based on value: a semantic 1% boundary becomes 0.01 on the wire. HSP
points instead use integer 0-100 on the wire. These are distinct field contracts.

Emergency Stop has a browser-local delivery path in addition to the normal Go
command bridge. When an already-connected browser receives the global Stop
event, it invalidates fetched command work, writes `hsp/stop` directly, and
blocks later motion writes until the backend publishes its authoritative Stop
command. This keeps Stop deliverable during backend loss without making the
browser a second motion planner. Disconnect and component teardown also attempt
a direct Stop before releasing the GATT session.

## UI Requirements

- Bluetooth controls are hidden unless the Bluetooth dispatch owner is selected or enabled in settings.
- Cloud REST credential controls are hidden or de-emphasized when Bluetooth is selected.
- If Bluetooth is selected and unavailable, the app reports that Bluetooth is unavailable; it does not silently use Cloud REST.
- Browser permission prompts are user-driven.

## Revisit Criteria

Reconsider native Go Bluetooth only after:

- cloud REST and browser-owned Bluetooth are stable
- the Go core has mature transport diagnostics
- a specific native packaging requirement justifies the complexity
- a prototype proves reliable Windows BLE behavior

## September 2026 reference review

The connection-specific `HandyBleSession` owns its immutable GATT pair,
serialized writer, unique request IDs, response waiters and notification listener.
A native write has a one-second deadline; failure fences further motion and
allows the owner's bounded Stop/disconnect path. Retiring a connection aborts
waiters and prevents old native promises or queued Stop from reaching a new one.
Optional clock-sync failure omits `server_time` rather than inventing an offset.
A successful sync uses the lowest round-trip sample; Play is timestamped at its
actual native write boundary. Write-only firmware remains supported: no new
mandatory firmware response wait is added to streaming commands. A correlated
late rejection, including a code-only error, ends the channel.

See [the review, references and simulation limits](bluetooth-intiface-review-2026-09-20.md).

## October 2026: the Coyote uses a worker

The DG-LAB Coyote 3.0 does not follow the browser-owned model. It has no
device-side buffer: the host must write a packet every 100 ms, and a hidden
tab both throttles timers and ends this bridge. AGENTS.md section 3 also allows
native Bluetooth in a worker process, so `bluetooth-worker` owns the Coyote's
radio while the core composes every packet. The Handy's Bluetooth path is
unchanged. See [ADR 0044](decisions/0044-coyote-estim-output.md).

## October 14, 2026: Handy can also use the shared worker

Native Bluetooth is a fourth Handy protocol selection alongside Cloud REST,
Browser Bluetooth and Intiface. It uses the computer running the Go application,
not the computer or phone displaying the UI. Selecting a protocol never opens
a radio connection. The backend owns discovery, readiness and connection lifetime.

`internal/ble` provides device-neutral links and version-3 IPC. One instance of
`bluetooth-worker` serves one profile and connection; a hung write cannot take
down another device's process. `internal/handyble` supplies the RPC driver while
`HandyBluetoothTransport` preserves the same HSP point mapping as the browser.
No frontend heartbeat or browser BLE lease is fabricated for a native connection.
Normal controller ownership and Emergency Stop rules still apply.

Native support uses firmware-4 RPC. Point chunks respect the negotiated ATT MTU,
and late replies cannot revive a retired stream or connection. A missing optional
clock reply leaves uncompensated, explicitly estimated playback timing. See the
[software review and remaining hardware checks](native-handy-bluetooth-review-2026-10-14.md).
