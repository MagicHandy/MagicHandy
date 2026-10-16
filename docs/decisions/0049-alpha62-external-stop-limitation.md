# ADR 0049: Alpha.62 scope while the Intiface Stop fix is upstream

## Status

Recorded 2026-10-15 for review with the alpha.62 release candidate. This records
the maintainer's direction to finish the Intiface smoothness comparison and
continue toward publication while the Stop safety work is in Buttplug's hands.
It does not close R23 or establish physical Stop acceptance for Intiface.

## Context

The installed Intiface Central 3.2.2+46 can acknowledge Stop for the Handy's
position-only feature without a hardware command or queue barrier. The defect
and a review candidate are public in
[Buttplug issue 971](https://github.com/buttplugio/buttplug/issues/971). The
candidate is not bundled or installed, and deferred command ordering remains
upstream work. An observed carriage stop at the end of a short run cannot
establish an emergency braking guarantee.

The separate sampler preference reduces unnecessary updates. Repeated unloaded
comparisons favored it for full sweeps, varying pace and Creative v2. The final
production-build full-sweep and Creative v2 pair was accepted by the operator.
That evidence resolves the tested smoothness issue, not the Stop defect.

## Release decision

Alpha.62 can carry the reviewed app changes without claiming an Intiface Stop
fix. R23 remains Critical and its upstream exit criteria stay open. Release
notes and the Handy Intiface settings identify the specific known limitation;
Native Bluetooth and Handy Cloud retain their device Stop paths. The settings
notice can be dismissed, following the existing UI convention.

The app must still cancel its producers and stale work, mark the shared engine
stopped, attempt every selected transport and report dispatch failures. Stop
remains outside routes and controller ownership. All safety, race, lifecycle,
architecture and release gates remain mandatory. No driver workaround, guessed
position hold, automatic reconnect, new motion path or modified Intiface binary
is included. This decision gives no assurance about loaded electrical output,
RF-loss decay, or third-party device behavior.

## Follow-up

Keep [the risk register](../risk-register.md#r23-emergency-stop-delivery-gaps)
and upstream issue open. After an upstream fix is available, verify device Stop,
queued-command ordering and observed unloaded behavior before removing the
limitation. Native worker crash recovery may still require disconnecting both
companions; vibrator output has simulation coverage only. These limitations
remain in the release evidence and notes.
