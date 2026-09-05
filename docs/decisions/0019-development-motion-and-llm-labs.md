# ADR 0019: Development motion and LLM laboratories

- Date: 2026-09-04
- Status: Accepted for development experiments; production control replacement pending physical acceptance

The build-only availability decision below is superseded by
[ADR 0021](0021-optional-labs-in-release-builds.md): every release now includes
Labs behind an app setting that defaults off. The motion/Stop contract,
preview-only LLM workflow and observation scope remain unchanged.

Update: [ADR 0020](0020-continuous-motion-library.md) promotes ten reviewed
continuous recipes and their shared compiler into the public library. The
panels, trial APIs, experiment prompts and atlas remain development-only.

## Context

Repeated changes to Creative's span profiles, reversal guides, interpolation,
timing fit and prompting have not resolved reports of robotic movement. In the
first Motion Lab, the user found directional timing and combined controls
jerky in one direction; anchoring improved the baseline only slightly. More
labels on the same generator would not independently test geometry, timing
and composition. Small local models also need a place to test simpler motion
contracts without altering production chat or starting a device.

## Decision

Development builds expose Motion Lab and a separate LLM Lab. The default
public build contains neither lab UI nor registered lab endpoints. A
`magichandy_labs` Go tag must be paired with the Vite `labs` build from the
same frontend. Tagged embedding selects the ignored `.labs-dist`; normal
embedding selects the canonical, committed `dist`. Build-marker tests reject
mismatched assets. Public route/start tests and linker inspection verify the
boundary; this is not a runtime preference or a hidden release menu.

Motion Lab includes an independent continuous carrier. A semantic `FlowSpec`
defines a band, minimum span, anchor, pace, variation time scale, reproducible
seed, up to four sections and up to three modulation layers. It does not use
`DynamicDefinition`, a named span profile or Creative's interval fitter.
Sections blend their controls in cycle space; layers modulate range, center
or pace within the carrier's bounds. Arbitrary position streams are not added
and then clipped.

The backend compiles this score into immutable position/velocity/acceleration
states. A private prepared-content field lets the existing motion plan retain
those derivatives. It is not deserializable from a client target. The shared
engine remains the sole owner of transitions, sampling, sanitization, live
limits, transport dispatch and Stop. Transport implementations do not change.
The normal sampler also rechecks the final quantized path after removal of
stationary wire edges, preserving easing without introducing another loop.

LLM Lab owns a bounded, in-memory conversation and one validated preview
score. It has separate control, sequence and layer contracts, editable
prompts, request-local model selection and optional schema-constrained output.
Each trial makes at most one provider call. Raw failures remain visible;
there is no repair or fallback. The backend applies only explicit, valid
fields to its preview. A successful reply never commands hardware. Audition
uses the existing controller, settings-fingerprint and Stop admission path.
Stop cancels generation and discards canceled work. Lab history is not
production chat history and is not persisted across app restarts.

The labs now occupy a dedicated development-only sidebar workspace at
`#/labs/chat`, `#/labs/motion`, and `#/labs/observations`. Previous Settings
bookmarks redirect there. Chat owns the main column and persistent composer;
setup, raw responses and comparisons are secondary disclosures. Switching lab
tabs retains visited editors and in-flight chat work. Leaving Labs cancels its
in-flight request; completed turns remain in the backend until reset or restart.

Observations are explicit, durable review records in the existing SQLite
`app_kv` table (`labs.observations.v1`), bounded to 200 records and 4 MiB of JSON.
They capture a motion score/method/limits or the exact LLM trial, including its
original prompt, raw response and limits. Stale conversation references are
rejected rather than attached to a different reply. Writes are transactional
and controller-gated. No observations are read by model prompting, motion
selection, preferences or training. “Use in chat” copies a record into an
editable draft; only Send submits it. The UI identifies the actual database
path and exports records on request. Public binaries register none of these
endpoints. See [the workspace guide](../labs-workspace.md) for lifecycle and use.

## Consequences and limits

The new architecture provides continuous variation without expanding a menu
of named textures, but a sinusoidal carrier is still a hypothesis about feel.
Experimental acceleration/jerk budgets may reduce achieved pace, especially
at high requested speeds. Score loops are finite and deterministic, and
layer periods are approximated to close the loop. Preview metrics are
commanded estimates before transport mapping, not carriage feedback.

Schema constraints improve syntax but do not guarantee intent or preservation
of unmentioned controls. Tests therefore score field changes and compilation,
and the UI labels accepted changes as “Preview updated,” with explicit changed
fields in response details. This does not claim physical acceptance. The currently
tested defaults use plain JSON for simple controls and schemas for sections
and layers. They are experimental defaults, not universal model claims.

The production Creative authoring framework remains available while physical
acceptance is open. Public builds retain the general shared-engine/provider
improvements but exclude the experimental panels and callable lab control
methods. No dependencies, native motion library or new transport were added.

See [the redesign review](../motion-lab-redesign-2026-09-04.md) for history,
physical trace evidence, model experiments, build commands and limitations.
