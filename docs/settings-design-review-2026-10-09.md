# Settings and setup visual review

This pass covers the shared presentation of General, Access, Device, Media,
Chat, Voice and Diagnostics, plus Easy and Custom setup. The provider branch
remains separate from `main` for later integration.

## Consultation and decisions

At the user's request, three consultations used Claude CLI with
`claude-opus-5-5 --effort high`. All completed successfully and returned that
model in their response metadata:

- Source review: 77,861 ms; tools disabled; supplied setup/settings JSX and CSS.
- Visual review: 47,425 ms; read-only access to two rendered screenshots of
  General and setup Welcome, without account identifiers or credentials.
- Tab review: 27,912 ms; the same screenshots; compared compact filled tabs
  against classic attached tabs after the user rejected underline indicators.

The first recommendation was to remove nested cards and repeated setup chrome.
The screenshot review then identified insufficient hierarchy in an entirely flat
form: headings resembled labels, helper text felt detached, and dividers did not
distinguish sections. The final treatment combines neutral heading bands with
flat section bodies, readable muted help and compact controls. Selection uses
fill and borders; informational notices use a separate inset surface. The third
consultation recommended the rectangular filled tab strip now used by Settings,
Chat/Access navigation and setup progress. No active-tab underline, moving line,
or left-edge tab indicator remains.

Accepted changes include:

- Remove setup's duplicate brand/sidebar and circular step markers. Keep labels,
  numbered progress, completed checks, disabled future steps and a fixed footer
  in long desktop steps; narrow layouts wrap labels and keep the footer sticky.
- Make setup choices a single radio list, with the recommendation next to the
  option title. Retain all choice descriptions at narrow widths.
- Use shared 34px desktop / 44px coarse-pointer controls, small rectangular
  corners, square checkboxes and visible keyboard focus. Scope changes to setup
  and settings so Emergency Stop and live controls retain their own treatment.
- Measure each ordinary field before aligning its label and control. Narrow
  fields inside account/model grids stay stacked; help never falls into an
  accidentally narrow label column.
- Put Check now beside the installed version. Keep visited help links in the
  app accent, group notice management with Interface, and use two columns for
  notification preferences when space permits.
- Fold inactive Labs, developer device options, media encoding controls and
  detailed script-timing explanation. Keep the short timing consequence visible.
- Consolidate duplicated ChatGPT response-speed guidance into one recommendation
  with an expandable explanation. Existing provider identities, saved routing,
  local retry, save timing and privacy consequences remain visible.
- Hide successful requirement summaries for unselected voice features. Partial
  and unmet requirements remain visible before selection.
- Move prompt composition below primary diagnostics tools and load it only when
  expanded. Closing cancels the request. Existing failures remain visible while
  the inspector is open, with Refresh available.

Some suggestions were deliberately not adopted: wholesale row-component
migration, hiding every description, shrinking body text, and per-section accent
colors. Existing semantic forms and backend-authoritative state are retained.

## Verification

The frontend suite passes 104 files / 804 tests, TypeScript and five-language
localization checks. Regression coverage verifies deferred prompt composition
and cancellation, preserves backend-exact copy/retry behavior, and checks voice
requirements before and after selection. Existing hosted/local setup, provider
selection, routing, saved state and permissions tests remain green.

Go full tests, race tests, vet, Windows and Linux-targeted lint and the pure-Go
shipping build pass. No Go production code, inference prompt, routing contract,
motion mapping or dependency changed. Payload measurements are in the
[goal scorecard](goal-scorecard.md); no new runtime RSS or latency benchmark is
claimed. The inspector removes a request from unopened Diagnostics.

Browser review covers the seven settings sections, Chat sub-navigation and Easy
and Custom setup with the existing hosted/local configuration. Normal, disabled,
read-only and model-readiness states were inspected. Phone-width checks retain
readable step labels, horizontal settings navigation and the permanent Stop.
Review uses a simulator with no device key. No hardware, network exposure,
account creation, media jobs or model installation was initiated.

The consultation is design feedback, not a novice usability study. Authenticated
account administration is covered by source/component tests; a new account was
not created solely for screenshots. Full device/voice installation and physical
motion are outside this visual pass. Runtime screenshots, CLI transcripts and
test logs remain in ignored `.scratch/`.

The final review runs at `http://127.0.0.1:50221/#/settings/general`, with setup
available at `#/setup/reconfigure`. Older review sessions remain running.
`check-review-llm.ps1` passes against ChatGPT / GPT-6 Sol. A production text-only
chat returned `2 + 5 = 7.` in 2022 ms with one provider call, no repair, no
semantic fallback and no motion. The installed Ollama Gemma backup completed
the real local readiness check through Custom setup; that check carried forward
to Finish. These are readiness observations, not comparative benchmarks.

At 390px, both setup and General have no horizontally overflowing form controls;
the settings strip scrolls its 415px of tabs inside a 292px viewport. All eight
Custom step labels remain present, and the permanent Stop remains within the
viewport. Temporary browser emulation was cleared after verification.
