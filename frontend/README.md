# Local Stroke Orchestrator — React UI

Frontend em **React 18 + TypeScript + Vite**.

## Desenvolvimento

Terminal 1 — API Python:

```powershell
cd c:\dev\git\Handy\local-stroke-orchestrator
pip install -e ".[dev]"
$env:PYTHONPATH="."
python -m app.main
```

Terminal 2 — Vite (proxy `/api` → `:8080`):

```powershell
cd frontend
npm install
npm run dev
```

Abra http://localhost:5173

## Produção (um único servidor)

```powershell
cd frontend
npm run build
cd ..
python -m app.main
```

Abra http://127.0.0.1:8080 (FastAPI serve `frontend/dist` + API).
