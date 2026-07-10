import { lazy, Suspense, type ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { StatusProvider } from "./contexts/StatusContext";
import { Layout } from "./components/Layout";
import { ConfigHub } from "./pages/ConfigHub";
import { ControlRoom } from "./pages/ControlRoom";
import { Freestyle } from "./pages/Freestyle";

const Library = lazy(() =>
  import("./pages/Library").then((m) => ({ default: m.Library })),
);
const ManualQueue = lazy(() =>
  import("./pages/ManualQueue").then((m) => ({ default: m.ManualQueue })),
);
const MouseControl = lazy(() =>
  import("./pages/MouseControl").then((m) => ({ default: m.MouseControl })),
);

function RouteFallback() {
  return (
    <div className="page" aria-busy="true">
      <p className="hint">Loading…</p>
    </div>
  );
}

function Lazy({ children }: { children: ReactNode }) {
  return <Suspense fallback={<RouteFallback />}>{children}</Suspense>;
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route
          element={
            <StatusProvider intervalMs={1500}>
              <Layout />
            </StatusProvider>
          }
        >
          <Route index element={<ControlRoom />} />
          <Route path="freestyle" element={<Freestyle />} />
          <Route path="hands-free" element={<Navigate to="/freestyle" replace />} />
          <Route
            path="controle-mouse"
            element={
              <Lazy>
                <MouseControl />
              </Lazy>
            }
          />
          <Route
            path="biblioteca"
            element={
              <Lazy>
                <Library />
              </Lazy>
            }
          />
          <Route
            path="fila"
            element={
              <Lazy>
                <ManualQueue />
              </Lazy>
            }
          />
          <Route path="config" element={<ConfigHub />} />
          <Route path="chat" element={<Navigate to="/" replace />} />
          <Route path="auto" element={<Navigate to="/freestyle" replace />} />
          <Route path="patterns" element={<Navigate to="/biblioteca" replace />} />
          <Route path="import" element={<Navigate to="/biblioteca?tab=import" replace />} />
          <Route path="personas" element={<Navigate to="/config" replace />} />
          <Route path="settings" element={<Navigate to="/config" replace />} />
          <Route path="sessions" element={<Navigate to="/config" replace />} />
          <Route path="diagnostics" element={<Navigate to="/config" replace />} />
          <Route path="device" element={<Navigate to="/" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
