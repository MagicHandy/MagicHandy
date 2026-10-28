# ADR 0062: Original Creative retired behind a setting; Creative v2 shown as Simple

Status: implemented, 2026-10-28, at the owner's request.

This amends the mode list of [ADR 0015](0015-selectable-llm-motion-modes.md)
and the user-facing name of [ADR 0024](0024-native-creative-strokes.md)'s
mode. Motion vocabularies, the shared engine, Autopilot and Stop are unchanged.

## Context

The Chat sidebar's **LLM motion** select offered Layered, Creative v2,
Creative, Pattern library and Off. Original Creative (`dynamic`) predates the
continuous contracts: it keeps its own strict schema, Autopilot and cloud
paths, and has no vibration level ([ADR 0061](0061-model-controlled-vibration-level.md)
offers one in Layered and Creative v2 only). Layered has been the new-install
default since alpha.72. The owner asked to remove original Creative, keep a
setting that restores it for now, and rename Creative v2 to Simple.

## Decision

1. **Original Creative is hidden by default.** The select offers Layered,
   Simple, Pattern library and Off. **Settings > Chat > Conversation > Offer
   the original Creative mode** (`llm.original_creative_mode`, off by default)
   adds Creative back after Simple. Its contract and paths are untouched, so
   turning the setting on restores the earlier behavior.
2. **Saved choices stay valid.** A profile whose saved mode is `dynamic`,
   including settings from before the selector, which still adopt it
   (ADR 0015), keeps the setting on: the chat keeps its mode and the select
   shows it. Turning the setting off while `dynamic` is selected moves the chat
   to Layered, the new-install default. `PUT /api/settings/llm-motion-mode`
   refuses `dynamic` while the setting is off and names the setting.
3. **Creative v2 is labeled Simple** wherever a person chooses or reads a mode:
   the Chat sidebar and its help text, the LLM Lab buttons and help, and the
   vibrator notice. Its description no longer calls it the default. The
   persisted and API identifier stays `creative_v2`, as do code names, ADR 0024,
   review documents and evaluation names, so the history stays searchable.

## Consequences

"Simple" also names the lowest model capability level
([ADR 0058](0058-model-capability-levels.md)), and that level has no vibration
level in Simple mode (ADR 0061). The two are separate settings in separate
places; the shared word is a known overlap to revisit if it confuses people.
Lab comparisons that include original Creative, such as the Motion Lab, are
unchanged. No motion permission, Stop path or saved motion changes.

## Validation

Config tests cover the hidden default, a saved `dynamic` keeping the setting
on, and turning it off moving the chat to Layered. HTTP tests cover the
refusal while hidden and the mode selection once restored. Frontend tests cover
the select without and with the setting and the LLM Lab's Simple button;
localization covers the five shipped locales.
