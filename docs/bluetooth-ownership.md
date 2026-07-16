# Bluetooth Ownership Decision

## Status

Initial decision: browser-owned Bluetooth bridge for early MagicHandy implementation.

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
inclusive 0-100 range. The browser encoder clamps that range but does not infer
or convert normalized 0-1 values; unit conversion by heuristic can turn an
intended 1% boundary into 100% motion.

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
