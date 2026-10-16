# ADR 0048: Device kit, device panel and Settings > Devices

## Status

Accepted 2026-10-15, option A of the device-management review. Implemented and
tested in simulation only. Replaces the connection-manager UI described in
[ADR 0045](0045-device-inventory-and-linked-sessions.md) and
[ADR 0047](0047-intiface-vibrator-output.md); their inventory, participation,
linked-session and lease decisions are unchanged.

## Context

The top-bar connection manager grew with every device
([ADR 0044](0044-coyote-estim-output.md), 0045,
[0046](0046-shared-native-bluetooth-worker.md), 0047). It mixed live jobs
(status, Connect, levels, limits) with setup: the Handy key, the method menu,
Coyote discovery and safety notes, Intiface pickers, calibration and status
artwork. Settings > Device repeated several of the same options behind the
shared draft and Save. Options had two homes, and each new device added a panel
with its own idioms.

## Decision

1. **Live jobs in the panel, setup in Settings.** The device panel holds each
   device's status, its one action (Connect, Disconnect, Set up or Reconnect),
   its In playback switch and its levels, then the speed/stroke limits and a
   Device settings button. Settings > Devices lists the devices, the playback
   failure policy and timing offsets, and the shared connections (Intiface
   Central and the Bluetooth helper). Each device has a page
   (`#/settings/devices/handy`, `/coyote`, `/vibrator`) with its setup
   sections and Remove. Every option has one home.
2. **One device kit.** `web/src/devices/` describes each device as data: its
   name, glyph, live model, page sections, timing offset and shared
   connection. The panel row, the top-bar chip, the Settings list and the
   device pages are generic renderers of it. A shell-level runtime provider
   owns the one browser Bluetooth bridge, per-device busy and failed state,
   and the faster status polls while a device surface is open.
3. **Immediate saves through a narrow patch.** `PATCH /api/settings/device`
   takes only the fields it names: the linked failure policy, the Intiface
   address, the API application ID source and override, clearing the
   connection key, the Coyote's estim settings and forgetting its remembered
   address, and the vibrator's settings. The backend applies the patch to the
   saved settings, normalizes and validates the whole document, and runs the
   same runtime transitions as a full save, so a failure-policy change still
   stops a running engine. It needs the controller and the host admission
   class. An empty or invalid patch is 400. A runtime failure after a
   successful save is 502 and says the setting was saved. The response is the
   client's redacted settings view, without the connection key. The key keeps
   its explicit Save (`PUT /api/settings/device/connection-key`).
4. **Server port in Settings > Access.** `PUT /api/settings/server` saves the
   port (1–65535), which applies at the next start.
5. **Method changes stay explicit.** A Handy connection method applies only
   through **Use this method**, which asks first while motion runs, because
   the backend stops motion before switching.

## Consequences

- Settings > Devices has no draft and no Save. The shared settings Save no
  longer writes the estim or vibration settings or the connection key.
- The connection manager, its method picker, the Coyote, vibrator, Intiface
  and native Bluetooth panels, the separate Coyote and vibrator settings, and
  the connection artwork are removed. `#/settings/device` opens Settings >
  Devices.
- A new device adds a descriptor and its transport, not a panel.
- The browser bundle change is recorded in the
  [goal scorecard](../goal-scorecard.md).

## Validation

- Go: patch application and validation, the patch and server-port handlers
  (controller, admission, redaction, runtime transitions) and the route
  admission matrix.
- UI: device models and status ranking, the panel (notices, watching tabs,
  empty state, levels), Settings > Devices and each device page, and the
  ported App and Settings tests, in five locales.
- A `-simulate-motion` review build at desktop and phone widths. Hardware
  acceptance is open.
