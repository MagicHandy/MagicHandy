# Reply latency review — 2026-11-10

## Question

Did recent releases make chat replies slower? If so, which change is
responsible, and should it be optional? Measurements use GPT-6.1 Terra through a
ChatGPT sign-in (reasoning low) and DeepSeek V4.1 Flash through OpenRouter.

## Installed history

Saved replies in the installed app's timing diagnostics cover October 25 to
November 1; later chats were unsaved and removed at exit. Terra's median reply
time was 3.9–4.5 s in two sessions on October 28 (alpha.77/78) and 6.7–6.9 s in
two sessions on October 31 (alpha.81/82). Nearly every reply in those sessions
changed motion. Prompts grew from about 22 KB to 26 KB and median generated
tokens from 158–198 to 202–245. These sessions had different requests, so they
show a trend but not its cause. No message text was read.

## Same requests on every release

A scratch benchmark sent the same eight requests through each release's own
prompt builder, alpha.77 to alpha.84, interleaved in shuffled order. There were
three motion edits, a counted motif, a question, praise, an invitation and
"Please, I want more", for 32 replies per release.

- **Input tokens grew 33%,** from about 5,970 to 7,960.
- **Output stayed flat** at about 100–115 tokens per reply (medians 96–114).
- **Reply time followed output length** (about 17 ms per output token,
  correlation 0.49) and showed no trend by release: medians ranged from 3.7 s
  to 5.4 s, and alpha.84 had the lowest.
- **The service dominated.** An earlier run of the same benchmark the same
  evening had medians of 9–13 s, and 46 of 320 calls failed with "cloud
  provider unavailable". The same request on one release took 4–15 s.

On identical cold, specific requests from the hosted suites, alpha.83 and the
alpha.84 guidance had the same median: 3.95 s and 3.98 s over about 355 requests
each.

## What alpha.84 changed

In roleplay, alpha.84's scene guidance
([ADR 0067](decisions/0067-advanced-chat-keeps-scene-reply-and-motion-in-step.md))
makes many more replies change motion. A reply that changes motion writes and
checks edits as well as words, so it takes longer. On Terra, "Please, I want
more" needed about 170–290 output tokens against about 35 for praise, and
motion-changing replies took about 1.5–2.3 times as long as text-only ones.

In the same 30 scenes at the same time on Terra:

| | Scene motion on (alpha.84) | Off |
| --- | --- | --- |
| Mean reply time | 4.21 s | 3.55 s |
| Median reply time | 3.30 s | 3.02 s |
| 90th percentile | 7.27 s | 6.68 s |
| Replies that change motion | 104 of 198 | 30 of 198 |
| Scene coherence (1–5) | 4.10 | 2.87 |
| Responsiveness (1–5) | 4.37 | 2.33 |
| Character fit (1–5) | 4.17 | 3.07 |
| Starts on the invitation | 24 of 24 | 3 of 24 |
| Eases off at the end | 24 of 24 | 0 of 24 |

DeepSeek V4.1 Flash showed the same pattern earlier: its roleplay median rose
from about 1.1 s to 1.9 s with the guidance.

## Change

See [ADR 0068](decisions/0068-quicker-replies-without-scene-motion.md). Model
roles gain **Answer roleplay cues with motion**, on by default, for Advanced
Chat models in Layered or Creative v2. Its hint states the trade: cues move the
device and replies describe only what it does, but replies that change motion
take longer, while off keeps replies quicker and motion changes mostly when
asked for. Off restores the earlier conversation rules for Advanced chat only;
the Advanced Stop action and every limit and gate are unchanged.

## Other levers

These already exist and are unchanged:

- **Response speed** on ChatGPT and other reasoning connections sets reasoning
  effort for every reply.
- **Automatically repair rejected chat responses** adds a second request only
  when a reply is invalid.
- **Prepared decisions** hide Autopilot planning time. The larger alpha.83
  planning prompts affect Autopilot only, not chat replies.
- **Advanced (no reasoning)** on a fast OpenRouter model such as DeepSeek V4.1
  Flash answers in about 1–2 s, at lower agreement than Terra.

## Not covered

- Time-to-first-token and streaming feel: the benchmark measured whole replies.
- Local Simple and Intermediate models, which never received the guidance.
- Real sessions after November 1, which were not saved.
