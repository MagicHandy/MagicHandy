# ADR 0067: Advanced chat keeps a scene's reply and motion in step

## Status

Accepted 2026-11-09. Advanced-capability live chat in Layered and Creative v2,
including Advanced (no reasoning). Simple and Intermediate prompts keep their
bytes. Persona reaction styles are unchanged and still never mention motion
([persona design](../persona-page.md) §2.3 and §3). See the
[roleplay review](../roleplay-flow-review-2026-11-09.md).

## Context

In roleplay the person rarely names a control. They invite, react, ask for
more, say they are close and wind down. Advanced chat read these turns as
conversation. Three prompt lines pushed it there:

- "A stopped device starts only for a direct motion request ... Ordinary
  conversation and questions require reply only."
- The final output rule: "Conversation requires "edits":{}".
- The live action guide used `none` for chat and kept `update` for "motion
  feedback", without saying that a reaction in a scene is motion feedback.

Meanwhile the warm, intimate and explicit voice checks ask for replies about
embodied action. The two pulled apart. On 24 voice and style presets plus six
roleplay types, GPT-6.1 Terra left the motion unchanged in 86% of turns. It
started on an invitation in 2 of 24 scenes and never eased off when the person
finished, while its replies described touch beginning, resting or holding
still. DeepSeek V4.1 Flash moved more but kept running at up to 100% while its
replies said "catch your breath", and described changes it did not make in
about a sixth of its claims.

## Decision

1. **A scene guide in Advanced live chat.** In a scene, the reply and the
   motion are one act. The person's reactions are motion feedback even when
   they name no control: an invitation to begin, pleasure, asking for more,
   saying they are close, finishing, or wanting to rest. The model answers in
   character with the motion as well as the words; saved limits apply as
   always. Praise usually means keep what works. Once the person finishes or needs to
   catch their breath, the model eases off or stops unless they ask for more.
   Holding them off means slower or lighter motion, not faster. Any change the
   reply describes (starting, stopping or resting, speeding up or slowing,
   going deeper or shallower, changing length) is made in the same turn, a
   reply never describes resting while the motion keeps its pace, and kept
   motion is not described as changing. Kissing or touching elsewhere is not
   the motion.
2. **Starting from stopped.** For Advanced chat a stopped device starts when
   the person asks for motion or invites the model to begin, never for a
   reaction to earlier motion. Paused motion still cannot be resumed by model
   edits.
3. **Empty edits.** Advanced chat uses empty edits for questions and talk that
   is not about the motion, rather than for all "conversation".
4. **Explicit Advanced live Stop.** Layered and Creative v2 offer `stop` with
   empty edits and no vibration change. It dispatches the existing shared Stop
   path, ends Autopilot and may also stop paused playback. Autonomous decisions
   do not acquire this capability. A later fresh manual motion request can start
   again.
5. **Unchanged.** Reaction-style and voice blocks, saved limits, capability
   gates, Emergency Stop and every Simple and Intermediate prompt.
   A vibrator-only session that cannot play does not receive the guide.

The guide is persona-agnostic and code-owned. A persona's style still shapes
only its words; the model may express that character through motion choices it
was already permitted to make when the person leaves the lead to it. There is
no keyword rule: reading a cue stays the model's judgment.

## Consequences

- Roleplay motion now follows the scene.
  - **Terra**, over two passes: coherence rose from 2.83 to about 4.0 (3.83 and
    4.23), responsiveness from 2.40 to 4.47 and character fit from 2.93 to
    4.22. It starts on an invitation in 48 of 48 scenes and eases off at the
    end in 47 of 48.
  - **DeepSeek V4.1 Flash**: coherence rose from 3.03 to 3.97, responsiveness
    from 3.73 to 4.87 and character fit from 3.63 to 4.10.
  - Creative v2 and chat with Autopilot composing improved by similar amounts.
- Advanced chat starts motion more readily: an invitation to begin now starts
  it. Reactions after a Stop still do not.
- Replies at the Advanced level describe stroking more often, so the explicit
  voice is more explicit in practice. Warm and intimate replies stayed free of
  explicit terms.
- Cold, specific requests are unchanged within run-to-run variation; see the
  review for the suites.

## Validation

`TestHostedChatKeepsSceneReplyAndMotionInStep` pins the Advanced-only guide and
wording, the unchanged Simple/Intermediate lines, and the vibrator-only exclusion.
`TestHostedRoleplayFlowLive` plays the preset matrix and roleplay types without
explicit requests, and `TestRoleplayFlowJudge` labels each reply from its text
alone, compares the labels with the compiled change and rates each scene.

The independent release review adds 27 real-provider lifecycle checks, ten
feedback-conversation turns, and fake-transport/race coverage for model-selected
Stop and fresh restart. See the release-review section of the evaluation report
for harness corrections, retained failures, latency and limits of the evidence.
