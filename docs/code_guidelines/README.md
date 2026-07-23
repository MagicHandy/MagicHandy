# Code Guidelines — MagicHandy

Guidelines for consistent, testable code in this repository.

## Principles

| Principle | Description |
|-----------|-------------|
| Single Responsibility | One reason to change per module; split large files (enforced by `file_size_test.go`) |
| Explicit over implicit | No hidden globals; inject `Runtime` collaborators into handlers |
| Fail fast | Validate at HTTP boundary and domain entry points |
| Testability first | Structural tests use fakes; **acceptance** requires real device + sync analysis (Rule 04) |
| Minimal scope | Smallest change that satisfies the feature doc; no drive-by refactors |
| Preserve invariants | HSP v4 rules and motion/transport contract (ADR-0002) are non-negotiable |

## Index

| File | Content |
|------|---------|
| [go.md](./go.md) | Go naming, packages, errors, concurrency |
| [typescript_react.md](./typescript_react.md) | Frontend components, hooks, API client |
| [clean_code.md](./clean_code.md) | Shared readability and change discipline |
| [architecture_layers.md](./architecture_layers.md) | Import rules and package placement |
| [design_patterns.md](./design_patterns.md) | Provider, adapter, and queue patterns in use |
| [local_http_server.md](./local_http_server.md) | httpapi handler conventions |
| [testing.md](./testing.md) | Real-device sync testing policy |

## Mandatory reads for AI

1. [`docs/context.md`](../context.md)
2. [`docs/adrs/0011-layered-architecture.md`](../adrs/0011-layered-architecture.md)
3. [`docs/adrs/0002-motion-transport-contract.md`](../adrs/0002-motion-transport-contract.md)
4. This directory before implementing any feature in `docs/tasks/features/`

## Related

- ADRs: [`docs/adrs/README.md`](../adrs/README.md)
- HSP invariants: [`docs/hsp-v4-invariants.md`](../hsp-v4-invariants.md)
- UI guidelines: [`docs/ui-design-guidelines.md`](../ui-design-guidelines.md)
