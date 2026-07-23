# General Guides — MagicHandy

Operational runbooks and flow documentation. Add guides here as the service matures.

## Available guides

| Guide | Description |
|-------|-------------|
| [README.md](../README.md) | Run from source, flags, health checks |
| [IMPLEMENTATION_PLAN.md](../../IMPLEMENTATION_PLAN.md) | Phase roadmap and status |
| [perf-baseline.md](../perf-baseline.md) | Memory measurement procedure and recorded baselines |
| [model-management.md](../model-management.md) | LLM model paths, load/unload, managed llama.cpp |
| [goals-and-guardrails.md](../goals-and-guardrails.md) | Measurable targets (RSS, binary size, cold start) |
| [risk-register.md](../risk-register.md) | Known risks and mitigations |
| [react-ui-implementation-handoff.md](../react-ui-implementation-handoff.md) | Frontend implementation notes |
| [procedural-chat-motion-analysis.md](../procedural-chat-motion-analysis.md) | Procedural motion deep dive |

## Planned guides

- `local-dev.md` — consolidated dev setup (Go + Node + llama.cpp)
- `real-device-validation.md` — retarget-validate workflow and trace export
- `deployment.md` — Phase 16 packaging (when available)
- `troubleshooting.md` — common transport/LLM failures

## Local dev quick start

```powershell
# Terminal 1 — backend
go run ./cmd/magichandy

# Terminal 2 — frontend hot reload (optional)
cd frontend && npm run dev
```

Default UI: `http://127.0.0.1:49717` (or Vite dev server on `:5173` with API proxy if configured).
