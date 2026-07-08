# MagicHandy + LSO Merge — Alternatives

Companion to [lso-merge-integration.md](lso-merge-integration.md). The merge has
several genuinely valid shapes. This doc lays out the options for each open
decision with trade-offs and a recommended default, so the team can choose
deliberately. Each decision, once made, should be recorded as an ADR under
`docs/decisions/`.

The recommendations lean on the project's stated priorities — device safety,
efficiency (RAM/CPU/binary), and maintainability — while keeping LSO's feature
depth. They are defaults to argue with, not verdicts.

---

## Decision 1 — Canonical frontend

Two React frontends now exist (a minimal shell and LSO's feature-rich UI), and
the binary can only ship one. Which is canonical, and what happens to the other?

- **A. Adopt LSO's frontend, retire the minimal shell.**
  - Pro: keeps LSO's feature depth (block editors, heatmaps, device surfaces,
    i18n) and the collaborator's frontend momentum; fastest to feature-complete.
  - Con: heavier browser footprint (the in-browser RAM cost was a core reason for
    the Go rewrite); must be brought under the design guidelines and a measured
    perf budget before it can be called canonical.
- **B. Keep the minimal shell, port LSO features into it incrementally.**
  - Pro: efficiency-first and guideline-clean by construction.
  - Con: re-implements work LSO already has; slowest to parity; risks losing
    LSO features in translation.
- **C. Adopt LSO's frontend but hold the line (hybrid). — Recommended.**
  - Make LSO's UI canonical, then bring it under `docs/ui-design-guidelines.md`
    (status-only bar, no round pills / glow / purple, one interactive hue,
    reduced-motion, accessibility) and set an explicit browser-perf budget
    (bundle size, idle memory, interaction cost). Trim or lazy-load features that
    do not earn their weight; keep the i18n table out of the critical path.
  - Pro: captures LSO's features while enforcing efficiency and the visual
    system; one clear canonical UI.
  - Con: requires a focused pass to reconcile styling and measure/trim cost.
- **Invariant regardless of choice:** exactly one UI ships and is embedded; the
  others are retired or folded in — never two frontends built and shipped in
  parallel.

---

## Decision 2 — Intiface / Buttplug transport scope

The merge adds a Buttplug transport. MagicHandy is currently HSP-only with no
fallback (ADR 0006).

- **A. First-class dispatch owner** alongside Cloud REST and Browser Bluetooth.
  - Pro: broadens device support to the whole Buttplug/Intiface ecosystem — a
    core LSO capability; already implemented purely behind the transport
    interface.
  - Con: widens the supported surface (three dispatch owners to keep safe,
    diagnosable, and tested); ADR 0006's "HSP-only, no silent fallback" scope
    must be formally revised, and owner-switch/safety-gate coverage extended to
    Intiface.
- **B. Opt-in / experimental**, off by default (setting or build tag).
  - Pro: keeps the default core lean and the primary path HSP; power users opt
    in.
  - Con: two support tiers; an experimental path still needs the safety gate.
- **C. Do not merge Intiface; stay HSP-only.**
  - Con: drops a defining LSO capability.
- **Recommended:** A or B, decided by how central Buttplug is to LSO's users —
  likely **A** if it is core — with a new ADR that supersedes ADR 0006's
  HSP-only language, and with Stop/owner-switch/goroutine-lifecycle tests
  extended to the new transport. The "no silent fallback, stop-and-report" rule
  applies to every transport.

---

## Decision 3 — Personalization model (personas vs prompt sets + memory)

LSO has personas (persona-driven chat + motion bias); MagicHandy has editable
prompt sets plus inspectable long-term memory.

- **A. Merge into one model. — Recommended.**
  - A persona becomes a prompt-set (behavior text) plus a motion-bias/style
    profile, over the existing memory system. One create/edit/enable surface;
    one code-owned motion-contract guarantee.
  - Pro: no overlapping systems to drift; one mental model for users and for the
    LLM contract.
  - Con: a mapping/migration pass from LSO personas to the unified model.
- **B. Keep both, clearly scoped.**
  - Con: two personalization systems tend to duplicate and diverge; harder to
    reason about what the model actually sees.
- **Invariant:** whichever, the motion JSON contract stays code-appended and
  cannot be edited out of a prompt/persona (Phase 10 rule); personalization stays
  inspectable and resettable.

---

## Decision 4 — Motion content (LSO blocks vs Pattern Library + arrangement)

LSO has a motion "block" library, editor, and queue; MagicHandy plans a Phase 14
Pattern Library and already has the Phase 11 bounded arrangement contract.

- **A. Fold LSO blocks into the Pattern Library + arrangement contract. —
  Recommended.**
  - Blocks become library content; playback and sequencing run through the shared
    engine and the arrangement contract.
  - Pro: one motion path (R14); one place for authoring, curation, and the LLM
    curation contract; no parallel block engine.
  - Con: adapt LSO's block format/queue onto the arrangement contract.
- **B. Keep LSO's library/queue as a separate playback path.**
  - Con: reintroduces the per-source motion divergence this project is explicitly
    avoiding (R14); protections added to one path miss the other.
- **Invariant:** no motion source bypasses the shared sampler/sanitizer.

---

## Decision 5 — Repository / integration shape

- **A. Single merged monorepo** (Go backend + one frontend + shared docs/CI). —
  **Recommended.**
  - Pro: one source of truth, one CI, one release; simplest for a small team and
    matches how the code is already landing.
  - Con: one repo's history carries both lineages (fine).
- **B. Shared Go backend, separate frontend repo.**
  - Pro: frontend and backend can version independently.
  - Con: cross-repo coordination, versioned API contracts, more release
    machinery than a two-person effort needs.
- **C. LSO stays a separate app consuming MagicHandy as a backend/service.**
  - Con: two apps to run and support; defeats the "merge" goal.

---

## Decision 6 — Localization pipeline

LSO ships a locale-generation toolchain and translation tables; MagicHandy has
localization docs and a prompt-localization strategy.

- **A. Adopt LSO's pipeline** as the generator, with one source-of-truth locale
  set and the existing docs describing wording/strategy. — **Recommended** if the
  pipeline is sound and its output stays out of the critical render path.
- **B. Keep the existing lighter approach** and port only the strings that are
  proven necessary.
- **Invariant:** one localization pipeline and one translation source of truth;
  generated locale artifacts are build output, not hand-maintained committed
  duplicates, and large untranslated-report/scratch files are not committed.

---

## Using this doc

Pick a default per decision, discuss in the PR that implements it, and record the
outcome as an ADR (`docs/decisions/NNNN-*.md`) so the choice is explicit and does
not have to be re-litigated. The [integration plan](lso-merge-integration.md)
sequences the work; this doc is the menu of shapes.
