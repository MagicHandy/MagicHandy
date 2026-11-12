# Autoblow Ultra and VacuGlide support — 2026-11-11

Status: implemented and tested in simulation
([ADR 0069](decisions/0069-autoblow-cloud-devices.md)). No Ultra or VacuGlide
was available, so every hardware question below is open. Sources are
Autoblow's developer documentation and support pages, read on 2026-11-10 and
2026-11-11 (see [Sources](#sources)).

## What the devices offer

Both devices are reached only through Autoblow's cloud:

- **Connection.** HTTP through regional clusters. A per-device token gives
  full control. `GET /<device>/connected` on `latency.autoblowapi.com` names
  the cluster, and every other request goes to that cluster. No LAN or
  Bluetooth API is documented.
- **Online mode.** Commands work only while the device is online; the
  VacuGlide's mode button, held for about 2.5 s, switches it. While online,
  button presses arrive as server-sent events instead of acting on the device.
- **Ultra.**
  - Go-to: position and speed, answered as soon as it is sent.
  - An oscillator between two points.
  - Position scripts, which Autoblow caps at 400 units per second and moves of
    at least 70 ms.
  - Stop is `oscillate/stop`.
- **VacuGlide.**
  - Target speed, 0–100%.
  - 30 built-in local scripts.
  - Speed scripts: each move's velocity becomes motor speed, 600 units per
    second is 100%, and the motor stalls below about 4%.
  - Stroke-plus and stroke-minus valves.
  - Stop is `target-speed/stop`.
- **Limits.** Requests are rate-limited per token. The Ultra reports its
  limit in headers; the VacuGlide's numbers are undocumented.

## The VacuGlide's construction: where depth and range come from

The user asked whether the two stroke valves are the VacuGlide's depth and
range control. They are, as far as public sources show:

- **Air moves the sleeve.** A crank-driven piston moves air through a hose
  into the receiver tube, and the air pulls and pushes the sleeve. Autoblow's
  adjustment page says the crank arm length (set with the supplied Allen key)
  sets how much air goes in and out, and limits the stroking distance. The
  VacuGlide 1 review describes the same key as a stroke limit from tip-only to
  near base-to-tip.
- **The valve changes how much air the sleeve holds.** Autoblow's valve page
  says the cap's valve "controls airflow", and its vacuum-seal page describes
  pressing the stroke button to suck air out of the system when air leaks in.
  A VacuGlide 2 review says the side valve sets how tight or long the stroke
  feels, and that built-in modes and Autopilot drive it to lengthen, tighten
  or pulse the suction. The Venus 2000, a similar pneumatic design, sets
  stroke length by the amount of air in its sealed system.
- **So stroke plus draws air out**, for a longer, tighter, deeper stroke, and
  **stroke minus lets air in**, for a shorter, looser one. Depth and length are
  one coupled axis. The device reports only whether each valve is open, never
  the air or the sleeve's position, so any automatic control is open-loop.
- **The video sync tracks tempo.** Autoblow says its VacuGlide sync follows
  the speed of the action, consistent with the speed-only API.

## Does MagicHandy's motion work 1:1?

**The Ultra: position targets are supported, not verified physical parity.**
Every stroke leg becomes one go-to. Commanded endpoints, holds and reversals
carry over, but actual travel speed and counted strokes need hardware testing. The shape within a
leg is the Ultra's own move. Legs can land a little early or late with the
cloud's jitter, and sparse curves keep the request rate near two per stroke.

**The VacuGlide: pace only, plus air.**

| Motion | The Handy | VacuGlide |
| --- | --- | --- |
| Pace and the Speed limits | yes | yes, as motor speed |
| Builds, slowdowns, bursts | yes | yes, averaged over about a stroke, one cloud trip late |
| Pauses | hold at a position | the motor stops; short turnaround dwells are smoothed away |
| Depth, stroke length, span, Stroke limits | yes | through the valves only: Stroke steps, or the experimental follower |
| Zones, anchors, stroke shape | yes | no |
| Counted strokes | yes | only as pace |
| Pattern library | full shape | each pattern's pace |

MagicHandy's 1–100% is 29–364 %/s of travel with the Handy Original reference
(32–400 mm/s over 110 mm). On Autoblow's scale that is about 5% to 61% of the
VacuGlide's motor: the slowest speeds sit at its stall point, and its top
speeds go unused on this reference mapping. Equal semantic travel does not
prove equal physical stroke rate on a pneumatic device. **Speed match** chooses between that
(the default) and stretching MagicHandy's top speed to the motor's.

## What was built

- **Settings › Devices.** Add device offers both devices. Each page has
  Connection: the device token with its own Save, online-mode help, the
  round trip, the firmware notice, the device buttons and the cloud Stop note.
  - The VacuGlide's page adds **Speed** (Speed match, the commanded motor
    speed) and **Stroke** (Shorter and Longer steps, and the experimental
    follower).
  - Both pages have Test: three gentle seconds while playback is idle.
- **Device panel.** Status, Connect and In playback for both devices. The
  VacuGlide's row adds a Stroke level (− shorter, + longer) and Test.
- **Playback.** Each device can play alone or in a linked session. Timing
  offsets are under Playback, and the visualizer shows percent when no Handy
  plays.
- **Stop.** Esc and Stop reach both devices through the cloud. The Ultra's
  pause button and the VacuGlide's mode button stop everything, and the speed
  buttons move the Speed limit's maximum by 10.
- **Model facts.** With only a VacuGlide, replies describe pace and rhythm,
  not depth or stroke length.

## Testing

All in simulation, against an in-process imitation of the API that accepts
any token and moves nothing:

- **Go tests** cover the client and its token redaction, both owners and
  sessions, the VacuGlide renderer, settings and HTTP admission, Emergency
  Stop, the device buttons, and the engine playing through each device alone.
- **Frontend tests** cover the device models, the VacuGlide page and the
  visualizer choice. The localization audit covers all five locales.
- **Full suites:** `go test ./...` and the frontend's 1,102 tests pass.
- **A `-simulate-motion` review build**, with GPT-6.1 Terra as the chat model:
  - both devices connected with test tokens;
  - the Ultra's Test oscillated and stopped by itself;
  - a chat request played through the VacuGlide alone: the reply was
    "Starting slow and steady." and the commanded motor speed was 17%;
  - Stop brought it to 0, and Disconnect stopped it before closing.
- **Fixes the review build found:**
  - the motion card drew a Handy in millimetres with only an Autoblow device
    playing;
  - the panel hid the stroke window while the Ultra, which uses it, played.

## Independent release review — 2026-11-12

The initial happy-path tests did not cover several network and lifecycle failures.
The release review reproduced and fixed redirect token exposure, queued valve
steps surviving Stop, a lost valve-open reply skipping closure, motor Stop
failure skipping valve cleanup, old playback resuming after reconnect, and
steady output surviving repeated health failures. Valve pulses are serialized
and bounded; both directions require the controlling administrator and current
Stop sequence. All cleanup stays bound to its admitted connection. Delayed
Stop confirmation cannot interrupt a fresh Play. Client errors and diagnostic
exports redact both device tokens, including encoded forms.

New regressions exercise both linked failure policies, reconnect staying idle,
concurrent valve limits, an uncertain test-start acknowledgement, and command
redirects. The UI labels support experimental and replaces the unverified
“same pace” promise with Reference pace. No physical device was connected.

## Hardware acceptance (open)

- **Ultra.**
  - go-to speed against Autoblow's 400 units-per-second scale;
  - how legs join over a real connection, and the offset that hides it;
  - the rate limit at about two requests per stroke.
- **VacuGlide motor.**
  - stroke rate at 5–100% against the 600 units-per-second scale, and the
    lowest reliable speed;
  - whether pace averaged over one second feels right.
- **VacuGlide valves.**
  - confirm stroke plus lengthens and tightens;
  - how much one 300 ms step changes the stroke, at rest and running;
  - whether the experimental follower helps, and the right step size.
- **Stop.**
  - time from Esc to rest on a good and a poor connection;
  - what happens after an app kill, a network loss and leaving online mode.
- **Buttons.** Whether they still act on the device while online; Autoblow's
  MCP guide and API reference disagree.
- **Coverage.** Whether the same API covers the VacuGlide 2 and the Ultra 2.

## Sources

Autoblow developer documentation:

- [Getting started](https://developers.autoblow.com/)
- [Vacuglide API V1](https://developers.autoblow.com/reference/http-api-v1-vacuglide/)
- [Autoblow API V1](https://developers.autoblow.com/reference/http-api-v1-autoblow/)
- [JavaScript SDK](https://developers.autoblow.com/guides/autoblow-js-sdk/)
- [MCP server](https://developers.autoblow.com/guides/mcp/)

Autoblow support pages:

- [Max stroke length](https://dev.autoblowme.com/vacuglide/adjustments)
- [Troubleshooting](https://autoblowme.com/vacuglide-2/troubleshooting)
- [Valve](https://autoblowme.com/vacuglide-2/valve)
- [Vacuum seal](https://autoblowme.com/vacuglide-2/vacuum-seal)
- [Porn sync](https://autoblowme.com/vacuglide-2/porn-sync)
- [Modes vs speeds](https://autoblowme.com/vacuglide-2/modes-speeds)

Reviews and background:

- [VacuGlide review](https://craftingworlds.com/slow-steady-unstoppable-why-the-autoblow-vacuglide-is-the-ultimate-no-frills-hands-free-toy/)
- [VacuGlide 2 review](https://craftingworlds.com/vacuglide-2-the-smartest-machine-to-ever-suck-you-off/)
- [US 5,501,650](https://patents.google.com/patent/US5501650), the Venus 2000
  design
