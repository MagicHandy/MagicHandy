# Multi-turn motion requests and conversation history

On the matched production chat tests, local Granite 3B passed **57/110** first-pass checks on the baseline and **61/110** with the selected changes. ChatGPT Terra Low passed **109/110** on both. The clearest local improvement was recalling a correction after the 64-message boundary. In the focused Granite 8B test, exact application and motion-free read-back each improved from **0/2 to 2/2**. Each denominator of two represents different names, endpoints and paces. The small focused samples establish what happened on these specific requests.

## What went wrong

The Advanced local settings promised the same bounded history as an external connection with the selected context size. The datastore loaded that history, but the interactive composer received empty message and byte limits and silently reapplied its 64-message default. At 86 prior messages it dropped 22, including the correction. At 230 it dropped 166. A separate Compact local read reported zero omissions after loading only the most recent 64 rows. Its model could confidently act on an unavailable reference.

The production change passes one history policy through storage and composition. Compact reads count omitted committed messages in the same SQLite statement as the selected dialogue. Composition counts prior user and assistant messages, excluding system guidance and the current request. The user’s configured context size and output budget still govern provider admission. [Storage and composition](../internal/httpapi/chat.go) · [End-to-end regression](../internal/httpapi/chat_window_regression_test.go)

Two English Layered guides earned their place on the local inference path. A Simple turn with missing history receives the current band and pace, an instruction to ask for an unavailable definition, and a reminder that a self-contained relative edit still applies. A running Advanced conversation longer than 64 visible messages receives a short example distinguishing descriptive read-back from a request to apply the recalled settings. Both guides sit immediately before the latest user message. The app admits a guide only if it fits the request budget with the dialogue intact. Paused, autonomous, manual-restart and non-position-control paths bypass these guides. [Local model guidance](../internal/chat/turn_score.go)

## Matched runs

The baseline is `16397a58`, which adds the evaluation harness to `d8188233` without changing production inference. Every row used the public SSE chat handler, canonical SQLite dialogue, actual provider output, the production parser and shared engine, and an in-memory transport. The dialogue was synthetic. The hosted fixture used MagicHandy’s protected ChatGPT sign-in in place.

- **Local** used Ollama and `huihui_ai/granite4.1-abliterated:3b`. Ordinary scenarios used the Simple vocabulary and the daemon’s normal allocation. Counted phrases used Advanced with a 65,536 application budget. Long recall used Advanced with 131,072. The 8B checkpoint received the focused long-history comparison.
- **ChatGPT** used `gpt-6.1-terra` through the production Responses provider, with Low reasoning and its Advanced vocabulary. Both providers used the utility voice and a saved speed range of 10–80. The baseline and candidate used the same authored requests and settings.
- Each model/build pair ran two variants of nine scenarios. The **110 turns** include 18 deterministic Stop requests. A pass requires a nonempty completion, the expected provider-call count, no repair or fallback, the right backend action, and the relevant target or preservation checks. Stops require a fresh transport acknowledgement. Geometry checks use the shared compiler’s commanded extrema, stroke lengths or regional turn coverage. A one-field edit is compared with the target actually playing before that request, even if an earlier selection failed.
- The corrected cues, competing pattern and follow-ups were real inferred turns. Between them the fixture inserted explicitly synthetic observatory dialogue into the same disposable session. The normal long run had 86 prior messages at the decisive turn. The larger test had 230. Provider token counts below show how much of the 131,072-token ceiling these requests used.

| Scenario | Granite 3B baseline | Granite 3B final | ChatGPT baseline | ChatGPT final |
| --- | ---: | ---: | ---: | ---: |
| Creative edits and variations | 7/14 | 7/14 | 14/14 | 14/14 |
| Layered starts and incremental edits | 4/12 | 4/12 | 12/12 | 12/12 |
| Apply a recently corrected cue | 8/12 | 8/12 | 11/12 | 12/12 |
| Apply a corrected cue after 86 messages | 6/12 | 6/12 | 12/12 | 12/12 |
| Shift a band or edit one endpoint, held-out wording | 7/12 | 6/12 | 12/12 | 12/12 |
| Edit and undo counted stroke groups | 3/12 | 4/12 | 12/12 | 12/12 |
| Read back a corrected cue without applying it | 6/12 | 8/12 | 12/12 | 12/12 |
| Ask for a cue outside the context window | 8/12 | 10/12 | 12/12 | 11/12 |
| Change the current speed after unrelated history | 8/12 | 8/12 | 12/12 | 12/12 |
| **All turns** | **57/110** | **61/110** | **109/110** | **109/110** |

Median end-to-end latency across model-backed turns was **0.400 s / 0.421 s** for local baseline/final and **3.793 s / 3.786 s** for ChatGPT. The two builds made 101/98 local provider calls and 92/93 ChatGPT calls across their respective 110 turns. Retries stay in both latency and failure accounting. Provider load, sampled outputs and trajectory realizations can vary between live runs. Individual one-turn differences occurred in both directions.

## Applying a corrected pattern and reading it back

The Harbor cue initially described 10–85 at speed 44. The user replaced it with **24–66 at overall speed 31**, started a different movement at 46, then asked to switch back after intervening conversation. Orchard used **19–73 at 37**. A separate run asked for the same three numbers as a read-back while the current motion continued.

| Delayed request, two cue variants | Baseline behavior | Final behavior |
| --- | --- | --- |
| Granite 3B, apply | The definition was absent. Both resulting paths still covered 0–100 with overall speed 46. | Both paths reached the corrected endpoints. The model encoded 31 or 37 as a relative group rate and left overall speed at 46. Both fail exact intent. |
| Granite 3B, read back | Returned the current movement’s numbers instead of the definition in both cases. | Both answers gave the three corrected numbers in order and made no motion change. |
| Granite 8B, apply / read back | Exact application 0/2 and motion-free factual read-back 0/2. | Exact application 2/2 and motion-free factual read-back 2/2. |
| ChatGPT, apply / read back | 2/2 for each request. Both definitions reached the model. | 2/2 for each request. |

The 86-message Harbor review makes the rate error visible. Mean commanded travel was **159.5%/s** on the previous 3B trajectory, **49.3%/s** on the new 3B trajectory, **119.4%/s** on the new 8B trajectory and **119.7%/s** on ChatGPT. The narrower 3B trajectory respected the endpoints while moving substantially more slowly. The numeric speed setting and the phrase’s rate multiplier are different controls.

At **230 prior messages**, Granite 3B admitted 64 on the baseline and 230 on the final build. Its final motion-free read-back was correct. Exact application still failed. ChatGPT admitted all 230 on both builds and passed both tasks. Actual prompts in that test were roughly 7.2K/14.6K input tokens on local 3B and 18.4K on ChatGPT. Granite 8B also missed both tasks on the baseline at this depth. On the final build it applied Harbor at 24–66 and overall speed 31, then in the separate read-back session returned all three numbers while the competing pattern kept playing.

When recent-only history deliberately excluded the cue, the baseline 3B changed the active plan in both variants. The final build held the plan in both, and both independent speed follow-ups applied correctly. Its spoken replies sometimes still announced a switch while maintaining the same plan. Terra asked for the missing definition in both variants. The missing-reference check scores the engine hold. Reading the replies separately exposed the wording discrepancy. The one final hosted miss in this sequence came on the independent speed follow-up, which required a repair.

## Prompt alternatives and held-out checks

One additional short conversation on the review app exposed a separate failure.
With motion enabled, Granite 8B started the simulator on “Please acknowledge
only and keep the simulator idle.” Its reply said the simulator was idle.
The subsequent correction also produced an update while the model described
keeping it idle. The transport trace shows the first command traversing 10–95%.
A text-only turn through the same app produced a reply and zero commands.
The simulator acknowledged Stop and issued zero additional commands during
the following 1.5 seconds. This independent smoke test is outside the matched
table. Its two stream plots and trace are under `.scratch/motion-context/review-atlas`
and `.scratch/motion-context/review-full-trace.json`.

- Extra explanatory prose gave Granite 3B **4/12** on the two ordinary Layered sequences, the same as its initial screen. Worked JSON examples gave **8/12**. Ordering the upper endpoint before the lower endpoint in compact-decoder examples gave **10/12** on that screen.
- Placing an example and the current score next to every request gave **11/12** on the development sequences. The later broad candidate scored **5/12** on independently worded mark-and-translate sequences against **7/12** on the matched baseline. Across eight independent cold-start questions and requests repeated twice, a starter example moved the result from 6/16 to 8/16, but still missed all four unfamiliar bounded-start phrasings. The shipped guidance is limited to the tested history conditions.
- A Creative v2 experimental schema gave the model named equivalents for full-only and local-only mix. It produced more correct mix selections, but could still stop five points short of the requested end or choose a roaming focus for a fixed-end mixture. The numerical Creative v2 vocabulary ships as before. Terra’s existing prompt matched or exceeded the hosted alternatives on the held-out requests.
- Counted groups remain difficult for Granite 3B. The final primary run passed **4/12**, including the two Stops. Terra passed **12/12**. The test scores the actual count, endpoints, group timing and overall pace.

## Visual evidence and reproduction

The atlas contains 1,865 evaluated turns, 825 distinct steady trajectories across 52 overview sheets, and 74 transport timelines from all completed experimental captures. It keeps rejected and held turns as records. The shared-engine plots overlay planned and quantized paths. Scenario-specific transport timelines show actual fake-owner appends through Stop. These are commanded estimates. Hardware feel and physical latency require a separate device session.

The baseline Granite 3B tip-only request produced a trajectory spanning 8.6–93.9% instead of the requested top 18 points. In the final mirrored counted sequence, Terra kept five short strokes at 12–42 and doubled the duration of the single 12–88 sweep. Its velocity plot shows the broad sweep below the peak speed of the short strokes. The largest finite-segment jerk across the experiments occurred in a rejected candidate’s 80% alternating sequence. The detailed plot shows continuous turns in both regions, with a maximum acceleration jump across a knot below 0.001%/s². The Stop timeline marks the queued tail canceled by the transport.

The ignored local evidence is under `.scratch/motion-context`. The atlas entry point is `atlas/index.html`, and `recall-comparison.png` compares the same Harbor request across builds and providers. The branch contains the fixtures and rendering tools. [Evaluation harness](../internal/httpapi/motion_quality_live_test.go) · [Visual review procedure](motion-visual-review.md)

With a running Ollama checkpoint, a clean worktree of `16397a58` supplies the comparable baseline. The same command on this branch exercises the final runtime. Each scenario uses its own in-memory transport and temporary database.

```powershell
New-Item -ItemType Directory -Force .scratch/motion-context | Out-Null
$env:MAGICHANDY_LIVE_MODEL = 'huihui_ai/granite4.1-abliterated:3b'
$env:MAGICHANDY_EXPERIMENT_CAPTURE = Join-Path $PWD '.scratch/motion-context/local-a.json'
$primary = '^TestLiveMotionRequestSequence$/^(creative|layered|recall_recent|recall_long|holdout|counted|recall_readback|recall_outside|long_unrelated)$'
go test -tags 'liveeval magichandy_labs' ./internal/httpapi -run $primary -v -count=1 -timeout 20m
```

Run `^TestLiveMotionRequestSequence$` to include the independent cold starts. Set `MAGICHANDY_QUALITY_MIRROR=1` for the second variant, or `MAGICHANDY_QUALITY_HISTORY_PAIRS=112` for the 230-message test. To run ChatGPT, set `MAGICHANDY_HOSTED_DATA_DIR` to an existing signed-in MagicHandy profile and set `MAGICHANDY_LIVE_MODEL=gpt-6.1-terra`. Use a new capture filename for each run. Live semantic failures intentionally make the test fail after it records the later requests and Stop. The ordinary CI suite uses deterministic fixtures.

```powershell
go run -tags magichandy_labs ./cmd/motion-atlas -catalog=false -llm .scratch/motion-context/local-a.json -output .scratch/motion-context/atlas.json
python scripts/render-motion-atlas.py .scratch/motion-context/atlas.json .scratch/motion-context/atlas --captured .scratch/motion-context/local-a.json
```
