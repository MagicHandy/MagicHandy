# Domain Rules — MagicHandy

Numbered business rules that implementers **must not invent**. When a rule conflicts with a feature doc, the feature doc wins for that feature's scope; otherwise these rules are normative.

## Index

| Rule | Topic | File |
|------|-------|------|
| 01 | LLM motion JSON contract | [01-motion-command-json-contract.md](./01-motion-command-json-contract.md) |
| 02 | Procedural vs library routing | [02-procedural-vs-library-routing.md](./02-procedural-vs-library-routing.md) |
| 04 | Real device + sync testing | [04-real-device-sync-testing.md](./04-real-device-sync-testing.md) |
| 05 | Procedural bridge filler | [05-procedural-bridge-filler.md](./05-procedural-bridge-filler.md) |
| 06 | Semantic intent + regiao map | [06-semantic-intent-zoning.md](./06-semantic-intent-zoning.md) |

## Format

Each rule file contains: **Rule**, **Rationale**, **Examples**, **Edge cases**.

## Related documentation

| Topic | Document |
|-------|----------|
| HSP position units, stroke window, reverse | [`hsp-v4-invariants.md`](../hsp-v4-invariants.md) |
| Motion vs transport semantics | [ADR-0002](../adrs/0002-motion-transport-contract.md) |
| Procedural chat deep dive | [`procedural-chat-motion-analysis.md`](../procedural-chat-motion-analysis.md) |
| Controller dispatch ownership | [`controller-dispatch-semantics.md`](../controller-dispatch-semantics.md) |
| Bluetooth ownership | [`bluetooth-ownership.md`](../bluetooth-ownership.md) |

## Adding rules

1. Pick next number (`04-slug.md`).
2. Follow the format of rules 01–03.
3. Update this index and `AI-GUIDELINE.md`.
