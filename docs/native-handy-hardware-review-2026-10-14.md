# Native Handy Bluetooth hardware connection check — 2026-10-14

The user authorized testing an original Handy (model 0), running firmware
**4.3.0+dcee7e58**, on Windows with a Qualcomm FastConnect 7800 Bluetooth adapter.
Connection keys, serial numbers, network details and hardware addresses are
excluded from this report. This follows the initial [software review](native-handy-bluetooth-review-2026-10-14.md).

## Verified

The isolated app at `http://127.0.0.1:50235/#/settings/device` used the real native
connector (`simulated=false`). Discovery and GATT validation succeeded, firmware
RPC replies were received, and clock synchronization completed. The usable ATT
write budget was **509 bytes**. The first successful app connection took
**9.95 seconds**, including the eight-second ambiguity scan; a later remembered
connection took **2.04 seconds**.

The final five-request Stop sample completed successfully: native dispatch
**79–155 ms, median 102 ms**; HTTP round trip **92–198 ms**. The visible Emergency
Stop button also succeeded and the connection stayed available. These are native
command-completion timings, not measured motor braking latency. No Play,
Add-points, stroke-window change or physical movement test was performed.

## Corrections prompted by hardware

- Initial HSP-specific Stop returned firmware error 100004 (wrong mode) while
  the Handy was in Idle. Native initialization, emergency Stop, disconnect and
  last will now use manufacturer RPC 715, StopCurrentMode, available from
  firmware 4.0.13. It stops the active mode without creating or playing a stream.
  Browser HSP golden fixtures are retained. A regression test rejects HSP-only
  and motion requests during initialization in another mode.
- Explicit WinRT disconnect raced the pinned library's disconnect callback,
  causing an access violation while releasing COM objects. The single-use
  Windows worker unsubscribes notifications and leaves final native-handle
  release to process exit. The core waits for exit or kills a hung helper.
  Subsequent observed worker exits were clean. Linux retains explicit disconnect.
- Failed discovery and empty characteristic lists are reported as discovery
  failures; they no longer incorrectly imply that compatible firmware lacks the
  required service. No profile requirement is relaxed.

## Remaining blocker

**Rapid disconnect/reconnect remains unreliable on this Windows host.** A local,
non-shipping diagnostic build exposed GATT **AccessDenied (3)** for characteristic
discovery even when RequestAccess reported Allowed (1). The pinned library omits
that result status and returns an empty list. The expected service and both
characteristics were verified separately; later independent connections succeed.

A short retry in a fresh worker did not fix this and was removed. Instrumentation
and temporary dependency copies are ignored local evidence, not shipping code.
Do not claim this is a firmware incompatibility or a solved timing problem.
Deterministic Windows GATT service/handle cleanup and repeated immediate reconnect
tests remain required before the draft PR is ready for integration.

The review instance has a verified working local Gemma model. Both devices were
disconnected during the APK-informed follow-up below. The older simulator remains at
port 50234. Ignored evidence under `.scratch/native-handy-hardware/` includes
`final-stop-samples.json`, `final-stop-trace.json`, and `final-diagnostics.zip`.
The export contains no user chat or copied credentials. The later browser-control
connection timed out, so there is no screenshot proof of the new COYOTE picker.

Motion quality, buffering/starvation, physical Stop time, radio/core/worker loss
during playback, and real concurrent Handy/COYOTE playback have not been validated.
The shared sampler, motion limits, HSP buffering and LLM behavior are unchanged.

## COYOTE 3.0 discovery and official APK inspection

The user confirmed COYOTE 3.0, disabled phone Bluetooth and placed the unit
near the PC antenna. A signal-ranked scan found an unnamed advertisement at
−24 dBm, 27 dB stronger than the next device. Read-only GATT inspection confirmed
GAP name `47L121000`, service `180C`, write `150A`, notify `150B`, and battery
`180A/1500`. The write characteristic exposes **0x04, Write Without Response**.
The advertisement itself has no name and contains an 11-byte manufacturer field.
Hardware addresses and private identifiers remain only in ignored local evidence.

The user supplied official DG-LAB 4.2.6 APK `dglab_online_426_0903_220556.apk`.
SHA-256: `ff68f0d4d6606a1f8a1d0b2d6b8bed684b8af45a0d95bcd906dbf15cdffc0c1a`.
Inspection was static: no installation or APK execution. JADX 1.5.6 from its
official release was used alongside extraction of the embedded JavaScript app.
The full decompilation had errors; the relevant native advertisement parser was
also inspected in simple mode and checked against the application bundle.

- Native scan code accepts the normal device-name families and unnamed
  manufacturer fields of 8–16 bytes as candidates. It retains name, address,
  RSSI and a private identifier. That byte length is not a unique device signature.
- The application resolves unnamed advertisements using a previously bound
  address or private-identifier prefix and stored model type. The binding flow
  can write an app-generated identifier on supported firmware. MagicHandy does
  not copy that write or alter the official app's binding.
- The official native write bridge uses Android Write Without Response. This
  agrees with the actual Windows GATT properties. A queued native zero cannot
  establish a confirmed strength baseline; the core now waits for a real report.

The resulting picker uses a separate bounded scan worker, returns host-only
opaque IDs rather than addresses, and ranks RSSI without auto-selecting. Labels
distinguish remembered devices, matching advertised names and possible unnamed
COYOTEs. Selected IDs expire after two minutes and are single-use. Selected and
remembered addresses never fall back to a different device. GATT validation is
required before limits or zero commands. The native byte protocol is now v4 so
an old helper cannot hide the absence of GATT acknowledgments.

The rebuilt app completed an eight-second scan in **8,152 ms**, returning two
possible unnamed candidates at −55 and −63 dBm. This was discovery only.
A subsequent test using the previously verified private address and the normal
worker/core COYOTE owner failed at characteristic discovery after **7,877 ms**;
no limits, zero, pulse or strength command was sent. A separate read-only
recheck returned GATT status 1 (Unreachable). The device's current readiness was
requested from the user. **Real battery, zero confirmation and Stop tests remain
blocked; no electrical output has been tested.**

Regression coverage includes private-address redaction, candidate identification
and ranking, explicit selection, expiry, no fallback to another device, scan-only
worker lifetime, unacknowledged-write capability propagation, and refusal to
increase strength or admit playback without a device report. Frontend validation
passes 901 tests in 117 files, TypeScript, all five locales and the production
build. Full Go and race suites, vet/lint (zero issues), architecture/lifecycle
checks and Windows/Linux CGO-disabled builds pass. The Windows portable-payload
check now recognizes the five deliberately shipped executables and passed under
PowerShell 5.1 and 7; the worker retains executable/header and vulnerability
checks. The exact review app completed a real local Gemma generation. The in-app
browser remains at Settings > Device, but browser automation repeatedly times
out before DOM/screenshot inspection; visual validation of the new picker remains
open. No third-party APK contents or runtime evidence are committed.

## Protocol and platform sources

- [Manufacturer RPC schema](https://gitlab.com/sweettechas/platform/handy-public-rpc):
  RequestStopCurrentMode field 715 and ERROR_WRONG_MODE 100004.
- [Windows GATT status](https://learn.microsoft.com/en-us/uwp/api/windows.devices.bluetooth.genericattributeprofile.gattcommunicationstatus): AccessDenied is value 3.
- [Windows GATT service cleanup](https://learn.microsoft.com/en-us/uwp/api/windows.devices.bluetooth.genericattributeprofile.gattdeviceservice.close): service resources must be released for subsequent access.
- [Pinned Bluetooth implementation](https://github.com/tinygo-org/bluetooth/tree/v0.16.0): WinRT disconnect callback and characteristic-discovery result handling.
