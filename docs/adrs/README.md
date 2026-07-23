# Architecture Decision Records — MagicHandy

All architectural decisions live in this directory. Index maintained for AI agents and contributors.

## Index

| ADR | Status | Summary |
|-----|--------|---------|
| [0001](./0001-go-first-core.md) | Accepted | Go-first core; Python only for optional ML workers |
| [0002](./0002-motion-transport-contract.md) | Accepted | Semantic intent vs physical transport separation |
| [0003](./0003-voice-worker-boundary.md) | Accepted | Voice ASR/TTS behind worker boundary |
| [0004](./0004-frontend-strategy.md) | Accepted | Fresh minimal-first UI rebuild |
| [0005](./0005-local-llm-runtime.md) | Accepted | Managed llama.cpp primary; Ollama secondary |
| [0006](./0006-drop-legacy-motion.md) | Accepted | Drop legacy motion path; engine + HSP only |
| [0007](./0007-voice-backend-selection.md) | Accepted | Parakeet, NeuTTS Air, ElevenLabs default stack |
| [0008](./0008-sqlite-persistence.md) | Accepted | SQLite for settings, memories, prompt sets |
| [0009](./0009-react-frontend.md) | Accepted | React + Vite frontend embedded in Go binary |
| [0010](./0010-project-structure.md) | Accepted | Repository layout and package boundaries |
| [0011](./0011-layered-architecture.md) | Accepted | Layered architecture and import rules |
| [0012](./0012-external-integrations.md) | Accepted | External service clients, credentials, timeouts |
| [0013](./0013-error-handling.md) | Accepted | HTTP error mapping and domain errors |
| [0014](./0014-testing-strategy.md) | Accepted | Real-device sync testing; structural CI as precondition only |
| [0015](./0015-director-actor-chat-latency.md) | Accepted | Director/Actor chat split for low TTFP procedural motion |

## Format

Each ADR follows: **Status → Context → Decision → Consequences**.

## Adding ADRs

1. Pick the next sequential number.
2. Create `NNNN-kebab-slug.md` in this directory.
3. Update this index.
4. Cross-reference from `docs/context.md` if project-wide impact.
