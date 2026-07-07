import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { StatusProvider } from "./contexts/StatusContext";
import { Layout } from "./components/Layout";
import { ConfigHub } from "./pages/ConfigHub";
import { ControlRoom } from "./pages/ControlRoom";
import { HandsFree } from "./pages/HandsFree";
import { Library } from "./pages/Library";
import { ManualQueue } from "./pages/ManualQueue";
import { MouseControl } from "./pages/MouseControl";

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
          <Route path="hands-free" element={<HandsFree />} />
          <Route path="controle-mouse" element={<MouseControl />} />
          <Route path="biblioteca" element={<Library />} />
          <Route path="fila" element={<ManualQueue />} />
          <Route path="config" element={<ConfigHub />} />
          <Route path="chat" element={<Navigate to="/" replace />} />
          <Route path="auto" element={<Navigate to="/" replace />} />
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
