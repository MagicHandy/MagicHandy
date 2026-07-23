# Tasks — MagicHandy

Task documentation for AI-governed development. **No production implementation without a corresponding feature or fix doc.**

## Structure

```
docs/tasks/
  README.md           ← this file
  TRACKER.md          ← central control point
  features/
    XXX-nome-kebab-case/
      TRACKER.md
      fase-N-descricao.md
  fixes/
    XXX-nome-kebab-case/
      ...
```

## Conventions

| Item | Rule |
|------|------|
| Feature/fix ID | 3 digits: `001`, `002`, … |
| Directory name | `XXX-kebab-case-slug` |
| Phase files | `fase-N-short-description.md` |
| Status legend | `[ ]` Pending · `[~]` In progress · `[x]` Done · `[-]` Cancelled |

## Branch and commit

- **Branch:** ticket id, e.g. `BEESCLO-18927`
- **Commit:** `feat(TICKET): English description` or `fix(TICKET): English description`
- Pull `main` before creating branch; ensure HEAD is 0\|1 commits ahead

## Workflow

1. Start session → read [`TRACKER.md`](./TRACKER.md)
2. Planning → use [`preprompt.criacao.tasks.md`](../prompts/DON'T%20READ/preprompt.criacao.tasks.md)
3. Implementation → use [`preprompt.executar.tasks.md`](../prompts/DON'T%20READ/preprompt.executar.tasks.md)
4. Complex bugs → use [`preprompt.bugfixing.complexo.md`](../prompts/DON'T%20READ/preprompt.bugfixing.complexo.md)

## Test commands

```powershell
go test ./...          # structural — not sufficient alone
go test -race ./...
cd frontend && npm run test
```

**Mandatory:** every test session uses a **real Handy device** with **synchronization analysis** ([Rule 04](../domain_rules/04-real-device-sync-testing.md)). Tasks are not done until device sync evidence is recorded.

## Related

- Project context: [`docs/context.md`](../context.md)
- AI index: [`AI-GUIDELINE.md`](../../AI-GUIDELINE.md)
