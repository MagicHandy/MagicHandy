# Creative v2 edits written without the response schema — October 9, 2026

## Report

A user on macOS reports Creative v2 turns rejected with `each Creative v2 edit
must name exactly one control group`. They run an MLX 4-bit build of the
recommended Gemma 4 12B model, and the model's reply looked correct in the
Lab. On alpha.57 a macOS MLX model can only be reached as an external
llama.cpp server, which talks to any OpenAI-compatible endpoint.

## Cause

Alpha.57's llama.cpp provider sends the Creative v2 response schema as
`response_format: {"type":"json_object","schema":…}`. That is a llama-server
extension. OpenAI-compatible servers read the standard
`{"type":"json_schema","json_schema":{"name":…,"schema":…}}` form instead: LM
Studio documents only that form, for GGUF and MLX models, and Ollama's
OpenAI-compatible endpoint treats `json_object` as plain JSON mode and drops
the schema. Without the schema, the model writes the contract unconstrained and
puts every requested control into one edit item, for example
`[{"range":{…},"speed_percent":60}]`. The parser rejected any item with more
than one key, although items are applied together and such an item has exactly
one reading.

## Reproduction

The production chat path (`Service.Complete`, Creative v2, motion running,
utility voice, reasoning off, production sampling) ran 14 requests three times
each against local Gemma 12B (`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`
through Ollama), under three output constraints. The requests mix compound edits
("Only the top half now, and faster"), single edits, a question and a hold.

| Output constraint | Valid at alpha.57 | Valid with this change |
| --- | ---: | ---: |
| Response schema enforced | 42/42 | 42/42 |
| JSON mode, schema dropped | 12/42 | 40/42 |
| No constraint | 12/42 | 40/42 |

Without the schema every compound request failed: 59 of the 60 rejections are
the reported error, and one reply wrote `edits` as an object. All 84
unconstrained replies were plain JSON, with no code fences and no partial groups.
For each request, the recovered replies name the same controls as the
schema-enforced replies. The four replies still rejected put a focus field
(`mix_percent`) beside `range` instead of inside `focus`; that is a partial
group without a single reading and stays rejected.

The two request forms were compared directly. On the managed llama.cpp runtime
(b11149) with the same model, both forms enforced the schema identically: 28/28
valid replies and no unconstrained shapes for each form over the 14 requests.
On Ollama's OpenAI-compatible endpoint, a schema requiring `{"color":"teal"}`
was ignored with the alpha.57 form (the prompt's `{"animal":…}` came back 3/3)
and honored with the standard form (3/3).

## Change

- The llama.cpp provider sends a schema in the standard `json_schema` form.
  Requests without a schema keep `json_object`.
- The Creative v2 parser splits an item holding several controls, or an object
  of controls, into one control per item before validation. Earlier-score
  recall reads the same shapes. Empty items carry no edit.

Duplicate or conflicting controls, unknown controls, partial groups, nulls, the
eight-item cap, saved limits, action authority, the single generation and the
absence of repair or fallback are unchanged. The prompt and schema still ask for
one control per item, which constrained decoding enforces. An accepted reply
compiles to the same score as its one-control-per-item form, so motion mapping
and character are unchanged and there is no new curve to render.

## Verification and limits

Regression tests use the recorded unconstrained Gemma replies, compare each with
its one-control-per-item form, cover recall in one item and as an object, and
run one through the HTTP chat path to the shared engine and fake transport.
`go test ./...`, `go vet ./...`, golangci-lint on the changed packages (zero
issues) and the `CGO_ENABLED=0` build pass; race tests are left to CI. Probe
replies, logs and the probe source are retained under
`.scratch/creative-v2-unconstrained-20261009/`.

The reporting user's server has not been identified. No MLX server or Apple
hardware was tested: LM Studio constrains MLX output with the standard form,
and a server without structured output now relies on the parser change alone.
