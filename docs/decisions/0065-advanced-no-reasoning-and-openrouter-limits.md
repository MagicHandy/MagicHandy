# ADR 0065: Advanced (no reasoning), connection output floors and OpenRouter host order

Status: implemented with live hosted evaluation, 2026-10-29. Amends
[ADR 0058](0058-model-capability-levels.md). Evidence:
[DeepSeek parity review](../openrouter-deepseek-parity-review-2026-10-29.md).

## Context

The owner asked whether DeepSeek V4.1 Flash 1017 over OpenRouter can match GPT
response quality, either with the existing prompt set or with another
capability level. They also asked whether the limits chosen with a model or
level should change. The earlier
[OpenRouter reasoning review](../openrouter-reasoning-review-2026-10-29.md)
found DeepSeek fast but less reliable than GPT 6.1 Terra.

The independent release audit counts 343 recorded production-turn intent checks
across 23 suites. Terra's composite baselines meet 318/343 and 315/343;
DeepSeek Advanced meets 270/343 and 280/343; two note revisions meet 302/343
and 297/343. Terra combines partial runs, and the note revisions are not
independent replications. The original Go-subtest accuracy totals were
misleading and are superseded by the linked audit.

- **DeepSeek's misses** were mostly unchecked rules and representation
  choices:
  - groups in separate regions written as strokes that break the loop seam;
  - pace by location written as an independent pace wave or as up/down pace;
  - `alternate_ends` widths over half the band;
  - unapplied requests re-applied;
  - requested feels declined;
  - repairs abandoned.
- **Reasoning did not close the gap.** With reasoning on, DeepSeek scored about
  the same as the planning note, at several times the latency. Its responses
  also needed far more room: 37-38% exceeded the 2,048-token OpenRouter
  allowance, and the largest reached 7.4K tokens.
- **The output limit truncated replies.** Connections inherit the global saved
  "Max output", whose factory default is 256 tokens. About 40% of DeepSeek's
  Advanced replies are longer than that, 1% exceed 512 and 0.6% exceed 1,024.
  The capability floors (Intermediate 512, Advanced 1,024) raised only the
  local model's saved limit. ChatGPT sign-in sends no output cap, which hid
  the problem.
- **Default routing was slow.** OpenRouter's balanced routing sent every traced
  request to a 4-bit host at an 8.6 s median. Sorting by throughput or latency
  chose a 0.9 s host with the same pass rates.

## Decision

1. **Add an Advanced (no reasoning) level.** Its ID is `advanced_no_reasoning`.
   - **Placement and scope.** It sits between Intermediate and Advanced in the
     selector. It has every Advanced feature, floor and budget, and the name
     says which models it suits: capable models answering without reasoning.
   - **The planning note.** Interactive Layered and Creative v2 chat add one
     required private `basis` string before `edits`, in the same generation,
     plus a closing guide. The guide covers four things:
     - it separates the current request from unapplied history;
     - it encodes a requested feel as motion rather than declining or asking
       for confirmation;
     - for Layered, it states the phrase-seam, `alternate_ends`-width, full-travel versus human-restricted
       band, and pace-by-location rules;
     - it asks repairs to keep the requested change in a valid form.
   - **Handling the note.** The parser removes the note before validation. It
     never becomes reply text, speech, memory or a motion command, and no length
     limit is advertised: strict schemas differ by provider in keyword support.
   - **Unchanged.** Autopilot, the reviewer, Simple, Intermediate and Advanced
     are unchanged byte for byte.
   - **Advanced hint.** The Advanced hint now suggests the new level when
     reasoning is off.
2. **Apply connection output floors per request.** A connection's capability
   floor applies to its own chat and Autopilot requests: 512 for Intermediate,
   1,024 for both Advanced levels. A larger saved limit is kept. The local
   model's saved limit and Simple connections are unchanged.
3. **Size the OpenRouter reasoning reserve by effort.** When reasoning may be
   active, the reserve is added to the answer budget:
   - 2,048 tokens for minimal;
   - 6,144 for low;
   - 8,192 for medium, high and above, and Provider default.
   - The reserve never exceeds a quarter of the known context, and the total
     is at least 2,048.
   - Off sends the answer budget alone.
4. **Add an OpenRouter host order.** Connections may send `provider.sort`:
   Fastest output (`throughput`), Fastest first token (`latency`), Lowest price
   (`price`), or OpenRouter balanced (no sort, the previous behavior).
   - New OpenRouter connections start at Fastest output, because motion waits
     for the complete reply.
   - Saved connections keep balanced routing until changed.
   - Other providers reject the field.
5. **Keep these unchanged.**
   - **Context floors.** The new level keeps Advanced's 64K floor. DeepSeek's
     prompts measured a 3.7K-token median, so the floor exists for history,
     not the contract.
   - **User motion and speed limits.** No model or level ever changes them.
   - **The default model.** It does not change, and nothing switches a
     connection's level by model name.
6. **Rejected alternatives.**
   - Worked examples helped phrase joins but lowered range and pace results.
   - Simplified contracts were already rejected in the earlier review.
   - Reinterpreting invalid phrases in the parser was rejected: it would turn a
     rejected proposal into a different motion.

## Consequences

The corrected historical intent checks favor the optional note over unchanged
DeepSeek Advanced, with roughly 0.8–0.9 s median latency versus Terra's 4.9–5.0 s.
This is not model parity: joins, unapplied history and repairs remain weak.
Fresh eleven-case production-path trials pass for both DeepSeek variants and
Terra, and two additional final-note trials pass. These small runs are evidence
of compatibility, not proof of a universal accuracy improvement.

The planning note adds tokens within one generation. Historical cached Together
calls cost about $0.0006 each. Other hosts, persona/explicit roleplay voice and
real-device playback were not measured in this release review. Saved limits,
validation, transport dispatch and Stop behavior are unchanged.
