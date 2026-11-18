# Motion requests on the default model profiles

The [installed-history follow-up](installed-chat-replay-2026-11-18.md) replays
retained conversations on these profiles and compares llama.cpp releases.

The baseline uses the application code at `72faf347` and the default-profile evaluation added at `0042748c`. Every measured turn used a real model, the ordinary streaming chat handler, SQLite dialogue, the production parser and the shared motion engine. The transport was an in-memory simulator. Scores measure the first response, the action the backend accepted and the trajectory it compiled.

| Setup selection | Provider and motion capability | Application context | Output and reasoning |
| --- | --- | ---: | --- |
| Gemma 4 E4B QAT Heretic, catalog SHA `063b60c663f5…` | Managed llama.cpp, Simple | 32,768 | 256 tokens, thinking off, catalog thinking-template fix |
| Gemma 4 12B Heretic Q4_K_M, catalog SHA `239ec3629639…` | Managed llama.cpp, Intermediate | 32,768 | 512 tokens, thinking off, embedded template |
| GPT-6.1 Terra | Signed-in ChatGPT Responses, Advanced | 131,072 | Low reasoning from the account catalog, provider-managed output |

These are fresh setup choices. Setup keeps the larger of the saved allocation and the capability floor. That gives E4B 32,768 tokens on a fresh profile even though Simple’s minimum is 16,384. The 12B run used the first, recommended catalog build for this 16 GB GPU. Both Gemmas used the app’s b11533 CUDA runner on an RTX 5070 Ti. All runs used the English utility voice and default 20–80% speed limits. [Local catalog](../internal/llm/model_catalog.go) · [Setup allocation](../web/src/util/setup-model-capability.ts) · [Hosted selection](../web/src/components/ModelResponseSettings.tsx)

## First-pass outcomes

| Model | All turns, baseline → final | Model-backed turns, baseline → final | Unrequested changes on holds, baseline → final | Provider calls, baseline → final |
| --- | ---: | ---: | ---: | ---: |
| Gemma E4B | 88/140 → 127/140 | 60/112 → 99/112 | 1/42 → 1/42 | 142 → 116 |
| Gemma 12B | 109/140 → 138/140 | 81/112 → 110/112 | 2/42 → 0/42 | 119 → 112 |
| ChatGPT Terra | 152/152 → 151/152 | 122/122 → 121/122 | 0/44 → 0/44 | 122 → 122 |

Each Gemma ran 140 turns, including 28 deterministic Stops. Terra also ran the Advanced counted-phrase sequence, for 152 turns including 30 Stops. The two primary variants contribute 110 local or 122 hosted turns. Each profile also had eight independent cold requests paired with Stop (16 turns) and two seven-turn restatement sequences (14 turns). Counts in different rows can include the same setup request. They describe these fixtures and both independently worded variants.

| Decisive request | E4B, baseline → final | 12B, baseline → final | Terra, baseline → final |
| --- | ---: | ---: | ---: |
| Bounded cold starts | 0/4 → 3/4 | 0/4 → 4/4 | 4/4 → 4/4 |
| Apply the corrected cue near the history limit | 0/2 → 2/2 | 0/2 → 2/2 | 2/2 → 2/2 |
| Read back the corrected cue with motion held | 2/2 → 2/2 | 2/2 → 2/2 | 2/2 → 2/2 |
| Ask for an unavailable definition with motion held | 2/2 → 2/2 | 2/2 → 2/2 | 2/2 → 2/2 |
| Read back the freshly restated cue | 2/2 → 2/2 | 2/2 → 2/2 | 2/2 → 2/2 |
| Apply that freshly restated cue exactly | 0/2 → 0/2 | 1/2 → 2/2 | 2/2 → 2/2 |
| Move only the lower endpoint | 0/2 → 0/2 | 0/2 → 0/2 | 2/2 → 2/2 |

The two main failures on the original 12B prompt were easy to miss by reading replies. “Full strokes between 5 and 95” repeatedly produced `full_slider`, which commands 0–100. When restoring a narrower corrected band, the model sometimes supplied its endpoints and speed but omitted `full_band`. The prior full-width stroke then exceeded the new band and validation rejected the edit. The Intermediate example now shows the paired range, geometry and overall speed as separate list entries. Its action reflects whether playback is stopped or running, and its example speed is bounded by the current settings.

On E4B, an optional-key object often emitted just the lower mark while the reply named both. Reordering the numeric example, adding a schema alternative for existing fields, and trying a dedicated sweep shortcut all left repeated endpoint omissions or unintended updates. The selected local Simple format reuses the existing grouped Layered range with two required endpoints. The same normalized score reaches the shared validator and sampler. Simple’s context, output budget and capability settings stay in force. The earlier object form remains readable. Intermediate also receives a Creative v2 reminder about end-anchored reach and exact repeats. E4B’s Creative trials did not consistently preserve requested holds, so its Creative path keeps its existing prompt.

For a long local turn the full example is optional. If it cannot fit, the composer tries a short cue and compares the resulting admitted dialogue with what the provider would have retained without it. The original request, recent dialogue, and declared motion authority remain intact. For example, in the 230-message read-back trial the person restates a correction near the end. The subsequent application still needs the new settings even though earlier conversation has been omitted. The full example alone gave 12B 0/2 exact applications in that test. The compact fallback gave 2/2 in the focused run and 2/2 again in the final matrix. E4B correctly read both fresh corrections but missed both applications. In each case it selected the corrected speed while the actual band stayed at 0–100.

Terra passed its first 138/138 baseline turns, plus 14/14 fresh-definition turns. Adding a local-style reminder yielded 137/138 on the matched primary cases; the missed turn was an unavailable provider response. There was no observed semantic gain to justify the additional prompt. The selected implementation retained the existing Terra prompt. In the fresh final run, one speed-only follow-up after reading back Orchard correctly reduced 46 to 39 but also changed the active 5–95 band to Orchard’s 19–73. Both builds used the same hosted prompt and model selection. All 44 final hold requests preserved the active plan.

**Remaining errors.** E4B made one unrequested update in 42 final hold turns when asked to repeat the exact Creative loop. On both late restatements it proposed updating the motion during an acknowledgement. Validation rejected those proposals and the app returned no successful acknowledgement. The two 12B misses were both lower-endpoint-only edits following a shift of the whole interval. The model’s reply promised to keep the upper endpoint, but the resulting trajectory did not satisfy that request. These are synthetic English chats and commanded estimates from a simulator.

In a separate cold start the person asked to touch 16 and 57 on **every** pass at speed 35. The baseline E4B plan covered 5–95 at speed 25. Baseline 12B reached both requested marks, but its mean stroke was only 30.4 points because it mixed in short strokes. Both final Gemma trajectories covered the full 41 points on every pass at speed 35. Terra met the request on both runs. The local comparison figure is `.scratch/default-models-20261118/default-model-trajectories.png`.

## What the harness checks

- A named profile binds the **real provider and the app settings together**. It locates each Gemma by the pinned catalog digest, applies its managed template fix, takes the recommended local capability and uses the same allocation rules as setup. Terra discovers its live account catalog and follows the new ChatGPT connection’s capability, context and reasoning selection.
- Every scenario keeps that profile. Exact counted phrases run on Advanced. Local profiles record the capability limitation and continue with the remaining scenarios. Local history is tested with 58 prior messages inside the window and 86 beyond it. The corresponding Terra tests use 86 and 270 messages. The separate restated definition arrives after 228 prior messages and is read back at 230.
- The fixture separately checks acknowledging a future definition, correcting it while idle, reading it aloud while a competing pattern plays, applying it, shifting both bounds, editing one endpoint, changing only speed, and Stop. A read-back must contain the requested numerals in order. For a deliberately omitted definition the user supplies a clarification to return. The harness checks the reply, the actual plan hold and the visibility of the source correction. A newly restated definition must still be available after truncation.
- Each capture retains raw model proposals, retries, backend actions, accepted geometry and the simulated transport commands. A successful repair is a first-pass miss. Stop requires a fresh transport acknowledgement. The local study atlas at `.scratch/default-models-20261118/atlas/index.html` retains baseline, experimental and selected runs, including failed selections and holds. Its detailed plots show whole-loop geometry, quantized points, velocity and acceleration. The captured transport charts mark Stop and the canceled queued tail.

## Reproduction

Set `MAGICHANDY_QUALITY_MODEL_DIR` to an app model-store data directory with the two catalog GGUFs installed, `MAGICHANDY_QUALITY_RUNNER` to the managed `llama-server` executable and `MAGICHANDY_HOSTED_DATA_DIR` to an existing signed-in profile. Hosted access uses the protected sign-in in place. Every chat uses its own synthetic session and simulated owner.

```powershell
python scripts/evaluate-default-motion.py --output .scratch/default-motion-review
```

The driver runs both numerical/wording variants, retains every capture and returns a failure status for unmet intent. To isolate a result, use `--profiles gemma-12b` and `--cases 'holdout|restated'`. A single profile is also available directly through `MAGICHANDY_QUALITY_PROFILE` and `TestLiveMotionRequestSequence` with `liveeval,magichandy_labs` tags. The earlier generic-model comparison remains reproducible. [Matrix driver](../scripts/evaluate-default-motion.py) · [Visual review procedure](motion-visual-review.md)
