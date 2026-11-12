# ADR 0068: Quicker replies without scene motion

## Status

Accepted 2026-11-10. Adds a setting to
[ADR 0067](0067-advanced-chat-keeps-scene-reply-and-motion-in-step.md); its
guidance and default behavior are unchanged. See the
[latency review](../reply-latency-review-2026-11-10.md).

## Context

Alpha.84 replies were reported to be slower. The retained measurements below suggest different contributors; they are
observations from one evaluation, not a controlled guarantee of latency:

- **Same requests, same moment.** On identical cold, specific requests, alpha.83
  and the alpha.84 guidance answered at the same median speed (3.95 s and
  3.98 s over about 355 requests each on GPT-6.1 Terra).
- **What the release changed.** In roleplay, alpha.84 makes many more replies
  change the motion (on Terra, 28 of 198 scripted turns before, about 106
  after). A reply that changes motion takes about twice as long as one that
  does not (about 6 s against 3 s on Terra), because the model writes and
  checks motion edits as well as words. The median roleplay reply rose from
  2.97 s to about 3.7 s on Terra, and from 1.1 s to 1.9 s on DeepSeek V4.1 Flash.
- **What the service did.** ChatGPT response times varied more than any release
  difference while this was measured: the same request on the same release
  took between 4 and 15 seconds in one evening.

The added prompt text (the scene and Stop guidance, about 300 input tokens) is
not isolated as a measurable cost in these samples. Across alpha.77 to alpha.84, the same eight
requests grew from about 5,970 to 7,960 input tokens while output stayed at
about 100–115 tokens per reply. Reply time followed output length, about 17 ms
per output token, without a clear release-related slowdown for these requests. Provider
variance and output differences limit causal conclusions.

## Decision

1. **A setting, on by default.** `llm.scene_motion_enabled`, shown as **Answer
   roleplay cues with motion** in Model roles when the Chat model is Advanced
   (including Advanced (no reasoning)) and LLM motion is Layered or Creative
   v2. Settings saved before it existed keep it on, so alpha.84 behavior is
   unchanged until someone opts out.
2. **What off does.** Live chat then omits the scene guidance and uses the
   earlier conversation rules: a stopped device starts only for a direct
   motion request, and conversation uses empty edits. The Advanced Stop
   action, the later-cue guidance, every other contract, limit and gate stay
   as they are.
3. **The hint states the trade.** Cues move the device and replies describe
   only what it does, but replies that change motion take longer. Off keeps
   replies quicker, and motion changes mostly when the person asks for it.

## Consequences

- People who prefer speed can opt out without changing model, level or
  reasoning effort. In the same 30 scenes at the same time on Terra, switching
  it off made roleplay replies about 0.7 s quicker on average (3.55 s against
  4.21 s; median 3.02 s against 3.30 s). It also returned alpha.83's
  weakness: scene coherence fell from 4.10 to 2.87 of 5, only 3 of 24 scenes
  started on an invitation and none eased off at the end, and replies again
  describe touch that the motion does not make.
- Simple and Intermediate models never received the guidance, so the setting
  is hidden for them.
- Reasoning effort ("Response speed" on ChatGPT connections) remains the
  setting for the cost of every reply. This setting covers only what scene
  motion adds.

## Validation

`TestSceneMotionDefaultAndUpdateRoundTrip` pins the default, the update round
trip and the copy semantics. `TestAdvancedChatWithoutSceneMotionUsesConversationRules`
pins the prompt with the setting off in both continuous modes, and
`TestChatCapabilitiesCarrySceneMotionChoice` pins the mapping from settings.
A frontend test pins where the switch appears and what it saves. The roleplay
evaluation (`MAGICHANDY_ROLEPLAY_SCENE_OFF=1`) compares both states.
