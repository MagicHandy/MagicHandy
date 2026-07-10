import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { MotionVisual } from "../api/types";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";
import { IconChevron } from "./icons/NavIcons";
import { mergeMotionVisual } from "../lib/mergeMotionVisual";
import { useFluidMotionVisual } from "../lib/useFluidMotionVisual";
import { useMotionEvents } from "../lib/useMotionEvents";

const OFFSET_PRESETS = [
  { label: "−160", ms: -160 },
  { label: "0", ms: 0 },
  { label: "+160", ms: 160 },
] as const;

function isPlaybackActive(
  visual: MotionVisual | null,
  snap: ReturnType<typeof useStatus>["snap"],
): boolean {
  return Boolean(
    visual?.playback_active ||
      snap?.manual_queue_playing ||
      snap?.playback_active ||
      snap?.direct_control_active,
  );
}

export function PositionVisualizer({
  variant = "panel",
}: {
  variant?: "panel" | "sidebar";
}) {
  const { t } = useTranslation();
  const { notify } = useToast();
  const { snap } = useStatus();
  const liveMotion = useMotionEvents();
  const [cachedVisual, setCachedVisual] = useState<MotionVisual | null>(null);
  const [offset, setOffset] = useState(-160);
  const [saving, setSaving] = useState(false);
  const [syncOpen, setSyncOpen] = useState(false);
  const loadingRef = useRef(false);
  const playbackWasActiveRef = useRef(false);
  const isSidebar = variant === "sidebar";

  const loadVisual = useCallback(async () => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    try {
      const v = await api.getMotionVisual();
      setCachedVisual(v);
      setOffset(v.offset_ms);
    } catch {
      /* */
    } finally {
      loadingRef.current = false;
    }
  }, []);

  useEffect(() => {
    void loadVisual();
  }, [loadVisual]);

  const visual = useMemo(
    () => mergeMotionVisual({ cached: cachedVisual, motion: liveMotion, snap }),
    [cachedVisual, liveMotion, snap],
  );

  const playbackActive = isPlaybackActive(visual, snap);

  useEffect(() => {
    const wasActive = playbackWasActiveRef.current;
    playbackWasActiveRef.current = playbackActive;
    if (playbackActive && !wasActive) {
      void loadVisual();
    }
  }, [playbackActive, loadVisual]);

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

  const polledPct =
    visual?.live_position_pct ??
    visual?.position_pct ??
    snap?.motion_position_pct ??
    50;
  const { pos, pathD } = useFluidMotionVisual(visual, polledPct);

  const min = visual?.stroke_min_pct ?? 10;
  const max = visual?.stroke_max_pct ?? 90;
  const measuredRtt =
    visual?.measured_rtt_ms ?? snap?.measured_rtt_ms ?? null;
  const gradId = isSidebar ? "vizGradSidebar" : "vizGrad";

  return (
    <section
      className={`glass visualizer-panel${isSidebar ? " visualizer-panel--sidebar visualizer-panel--pro" : ""}${playbackActive ? " visualizer-panel--live" : ""}`}
    >
      <div className="viz-head">
        <div className="viz-head-main">
          {isSidebar && playbackActive && (
            <span className="viz-live-badge">
              <span className="viz-live-dot" />
              {t("layout.visualizer.live")}
            </span>
          )}
          <span className="section-label">
            {isSidebar ? t("layout.visualizer.position") : t("layout.visualizer.positionHandy")}
          </span>
          {!isSidebar && (
            <p className="hint">{t("layout.visualizer.realtimeHint")}</p>
          )}
        </div>
        <div className="viz-pos-readout mono" aria-live="polite">
          {pos.toFixed(0)}
          <span className="viz-pos-unit">%</span>
        </div>
      </div>

      <div className="viz-body">
        <div className="viz-track">
          <div
            className="viz-stroke-band"
            style={{ bottom: `${min}%`, height: `${max - min}%` }}
          />
          <div
            className="viz-marker"
            style={{ bottom: `${pos}%` }}
            aria-hidden
          />
        </div>
        <svg className="viz-spark" viewBox="0 0 100 100" preserveAspectRatio="none">
          {pathD && (
            <path
              d={pathD}
              fill="none"
              stroke={`url(#${gradId})`}
              strokeWidth="1.5"
            />
          )}
          <defs>
            <linearGradient id={gradId} x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor="#6366f1" />
              <stop offset="100%" stopColor="#a78bfa" />
            </linearGradient>
          </defs>
        </svg>
      </div>

      {isSidebar ? (
        <div className="viz-sync-pro">
          <button
            type="button"
            className="viz-sync-toggle"
            onClick={() => setSyncOpen((o) => !o)}
            aria-expanded={syncOpen}
          >
            <span>
              Sync offset
              <strong className="mono">{offset} ms</strong>
            </span>
            <IconChevron className="viz-sync-chevron" open={syncOpen} />
          </button>
          {syncOpen && (
            <div className="viz-sync-panel">
              <label className="field sync-field">
                <input
                  type="range"
                  min={-500}
                  max={500}
                  step={5}
                  value={offset}
                  disabled={saving}
                  onChange={(e) => setOffset(Number(e.target.value))}
                  onMouseUp={(e) =>
                    saveOffset(Number((e.target as HTMLInputElement).value))
                  }
                  onTouchEnd={(e) =>
                    saveOffset(Number((e.target as HTMLInputElement).value))
                  }
                />
              </label>
              <div className="viz-sync-presets">
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  onClick={autoSync}
                >
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
                  RTT {measuredRtt ?? "—"} · dev {visual?.device_latency_ms ?? "—"}{" "}
                  · cli {visual?.client_latency_ms ?? "—"} ms
                </p>
              )}
            </div>
          )}
        </div>
      ) : (
        <div className="sync-controls">
          <label className="field sync-field">
            <span>
              Offset sync: <strong className="mono">{offset} ms</strong>
            </span>
            <input
              type="range"
              min={-500}
              max={500}
              step={5}
              value={offset}
              disabled={saving}
              onChange={(e) => setOffset(Number(e.target.value))}
              onMouseUp={(e) =>
                saveOffset(Number((e.target as HTMLInputElement).value))
              }
              onTouchEnd={(e) =>
                saveOffset(Number((e.target as HTMLInputElement).value))
              }
            />
          </label>
          <div className="btn-row">
            <button type="button" className="btn btn-ghost btn-sm" onClick={autoSync}>
              Auto-sync
            </button>
            {OFFSET_PRESETS.map((p) => (
              <button
                key={p.ms}
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => saveOffset(p.ms)}
              >
                {p.label} ms
              </button>
            ))}
          </div>
          {(measuredRtt != null ||
            visual?.device_latency_ms != null ||
            visual?.client_latency_ms != null) && (
            <p className="hint mono">
              RTT {measuredRtt ?? "—"}ms · device{" "}
              {visual?.device_latency_ms ?? "—"}ms · client{" "}
              {visual?.client_latency_ms ?? "—"}ms
            </p>
          )}
        </div>
      )}
    </section>
  );
}
