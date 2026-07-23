import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import { useToast } from "../contexts/ToastContext";
import { usePositionVisual } from "../contexts/PositionVisualContext";
import { TopbarMenu } from "./TopbarMenu";

const OFFSET_PRESETS = [
  { label: "−160", ms: -160 },
  { label: "0", ms: 0 },
  { label: "+160", ms: 160 },
] as const;

export function SyncOffsetMenu() {
  const { t } = useTranslation();
  const { notify } = useToast();
  const {
    offset,
    setOffset,
    saving,
    setSaving,
    loadVisual,
    measuredRtt,
    visual,
  } = usePositionVisual();

  const saveOffset = async (ms: number) => {
    setSaving(true);
    try {
      await api.setSyncOffset(ms);
      await loadVisual();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setSaving(false);
    }
  };

  const autoSync = async () => {
    try {
      const r = await api.autoSync();
      notify(t("layout.visualizer.offsetSaved", { ms: r.offset_ms, rtt: r.measured_rtt_ms }), "ok");
      await loadVisual();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <TopbarMenu
      label={t("config.settings.connections.sync.title")}
      detail={`${offset} ms`}
      align="right"
    >
      <div className="menu-panel-section">
        <span className="section-label">{t("config.settings.connections.sync.offset")}</span>
        <p className="hint menu-hint mono">{offset} ms</p>
        <label className="field sync-field">
          <input
            type="range"
            min={-500}
            max={500}
            step={5}
            value={offset}
            disabled={saving}
            onChange={(e) => setOffset(Number(e.target.value))}
            onMouseUp={(e) => saveOffset(Number((e.target as HTMLInputElement).value))}
            onTouchEnd={(e) => saveOffset(Number((e.target as HTMLInputElement).value))}
          />
        </label>
        <div className="viz-sync-presets">
          <button type="button" className="btn btn-ghost btn-sm" onClick={autoSync}>
            Auto
          </button>
          {OFFSET_PRESETS.map((p) => (
            <button
              key={p.ms}
              type="button"
              className={`btn btn-ghost btn-sm${offset === p.ms ? " active" : ""}`}
              onClick={() => saveOffset(p.ms)}
            >
              {p.label}
            </button>
          ))}
        </div>
        {(measuredRtt != null ||
          visual?.device_latency_ms != null ||
          visual?.client_latency_ms != null) && (
          <p className="hint mono viz-sync-metrics">
            RTT {measuredRtt ?? "—"} · dev {visual?.device_latency_ms ?? "—"} · cli{" "}
            {visual?.client_latency_ms ?? "—"} ms
          </p>
        )}
      </div>
    </TopbarMenu>
  );
}
