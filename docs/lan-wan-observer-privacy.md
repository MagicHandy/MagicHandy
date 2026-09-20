# Remote observation and host diagnostics

This is the response-audience contract for the in-progress self-hosted LAN/WAN
implementation. Accounts share one installation's conversation and library;
they do not create separate content tenants. The contract supplements the
[route admission matrix](../internal/httpapi/testdata/route_admission.tsv).

## Shared operations and private host data

An enabled operator may observe shared semantic motion, playback state, library
content, conversation text and voice progress. An unexpired administrator-issued
control grant additionally admits ordinary playback, live limits, LLM motion-mode
selection, chat and pattern feedback through the existing controller/delivery
checks. It does not authorize host configuration, model/module management,
filesystem operations or diagnostics.

`PublicSettings` removes stored credentials but is insufficient for remote
observation: it includes host paths, worker arguments and private service URLs.
The HTTP layer now selects the fields appropriate to each audience:

| Response | Observer/operator view | Administrator view |
| --- | --- | --- |
| App/settings | UI preferences, semantic motion/Autopilot settings, playback filters, selected provider, motion capabilities and voice input/output preferences | Existing credential-redacted configuration and load status |
| LLM | Availability and loading/readiness flags | Model IDs, service location and diagnostics |
| Voice | Worker lifecycle, queue counts, request IDs, transcript and audio progress | Worker command, module inspection and raw worker diagnostics |
| Media | Shared titles, catalog IDs, playback metadata and scan/job counts | Host roots, scan locations, filenames and failure details |
| Motion/transport/sync | Semantic engine state and operational connection/latency flags | Last transport result and raw failure details |
| Chat stream/history | Text, semantic motion, sequence/revision/cursor metadata and speech correlation | Provider/model/prompt metadata and inference diagnostics |

Empty fields can remain in typed JSON responses for compatibility; they must
not be filled with host values. Error details are replaced with a localized
administrator-directed message. Stop cancellation and known recovery errors
retain actionable shared wording. Conversation text, persona/prompt/memory
content and library exports remain shared installation content. This is an
explicit shared-content policy, not per-account isolation.

Detailed Labs, trace, prompt-diagnostic and transport-inspection routes require
administrator admission. The 207-registration table now contains 119 host,
34 shared, 30 semantic-control, 14 self-service, 6 public and 4 gateway entries.
Implicit HEAD follows the same policy. Physical Handy model/calibration changes
within the quick-settings endpoint also require administrator access; a rejected
mixed patch cannot partially apply its semantic fields.

The projections live in focused `internal/httpapi/client_*.go` files. Shared
chat events allow explicitly reviewed fields and reject unknown event shapes.
New payload fields and event types require an audience decision. Domain stores
and the shared motion engine do not become aware of HTTP account roles.

## Public Stop and browser lifetime

Protected installations return a minimal Stop acknowledgement to every caller,
including a caller supplying a cookie. The public lane deliberately performs no
login/database admission. Its response includes local stop state and whether
the transport confirmed Stop; it contains no previous target, settings or raw
transport failure. Failed physical confirmation remains explicit. The shared
engine still stops even when transport delivery fails. Unprotected loopback
installations retain the existing local response.

The production browser provider tree remounts when the login changes or ends.
Obsolete reads and streams are canceled, private settings drafts disappear, and
stored notifications are replaced for the new login audience. Pending quick
edits can flush on a route change within the same login, but cannot flush after
that login's provider tree ends, including when an earlier request finishes late.
Server authority/generation checks remain independently required.

Settings and Labs bookmarks show the administrator boundary without mounting
host tools for operators. Own sign-in management remains accessible. Persona
editing, media scanning/conversion/cover writes and library administration are
disabled for operators while granted playback, duration updates and feedback
remain available. Settings waits for the backend capability snapshot before
mounting host settings panels. Stop remains outside these route restrictions.

## Regression evidence and limits

Tests exercise real settings/state/voice handlers with authenticated roles,
sentinel private configuration and implicit HEAD, and adversarial typed runtime
views containing private failures. A granted operator completes one scripted
chat without repair/fallback, receives committed text/revisions, and cannot see
the model diagnostics retained for the administrator. A fake engine verifies
public Stop privacy; a failed confirmation retains its honest response.

Frontend regressions exercise the production login provider tree, delayed
responses, notifications and queued quick edits across login changes. Role
tests cover settings bookmarks, disabled Labs, physical device calibration,
media browsing, playback duration and pattern playback/feedback versus host
writes. These tests supplement admission coverage; they do not establish an
exhaustive handler/resource authorization audit or actual WAN/device behavior.

Build, runtime and artifact evidence is recorded in the
[implementation log](lan-wan-implementation.md) and [scorecard](goal-scorecard.md).
Owner-approved invitations, bounded audit history, stronger WAN enrollment,
network fault/load and real mobile/device acceptance remain part of the full
[checklist](lan-wan-control-checklist.md).
