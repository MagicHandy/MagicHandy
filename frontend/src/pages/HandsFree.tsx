import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import { SessionRail } from "../components/SessionRail";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";

type SignalId = "more_intensity" | "less_intensity" | "edging" | "climax";

export function HandsFree() {
  const { t } = useTranslation();
  const { snap, error } = useStatus();
  const { notify } = useToast();
  const [busy, setBusy] = useState(false);
  const [signalBusy, setSignalBusy] = useState<SignalId | null>(null);
  const [queueDetail, setQueueDetail] = useState<
    Awaited<ReturnType<typeof api.getQueue>> | null
  >(null);

  const signals: {
    id: SignalId;
    title: string;
    hint: string;
    urgent?: boolean;
  }[] = [
    {
      id: "more_intensity",
      title: t("handsFree.signals.moreIntensity.title"),
      hint: t("handsFree.signals.moreIntensity.hint"),
    },
    {
      id: "less_intensity",
      title: t("handsFree.signals.lessIntensity.title"),
      hint: t("handsFree.signals.lessIntensity.hint"),
    },
    {
      id: "edging",
      title: t("handsFree.signals.edging.title"),
      hint: t("handsFree.signals.edging.hint"),
      urgent: true,
    },
    {
      id: "climax",
      title: t("handsFree.signals.climax.title"),
      hint: t("handsFree.signals.climax.hint"),
      urgent: true,
    },
  ];

  const active = Boolean(snap?.hands_free_active);
  const intensity = snap?.intensity ?? 50;

  const refreshQueue = useCallback(() => {
    api.getQueue().then(setQueueDetail).catch(() => setQueueDetail(null));
  }, []);

  useEffect(() => {
    refreshQueue();
    const id = setInterval(refreshQueue, 4000);
    return () => clearInterval(id);
  }, [refreshQueue, snap?.playback_active, snap?.queue_blocks]);

  const toggleAuto = async () => {
    if (busy) return;
    setBusy(true);
    try {
      if (active) {
        await api.stopHandsFree();
        notify(t("handsFree.stopped"), "info");
      } else {
        await api.startHandsFree();
        notify(t("handsFree.started"), "ok");
      }
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const sendSignal = async (id: SignalId) => {
    if (!active || signalBusy) return;
    setSignalBusy(id);
    try {
      const res = await api.handsFreeSignal(id);
      if (id === "edging") {
        notify(t("handsFree.edgingDone", { count: res.enqueued ?? 0 }), "ok");
      } else if (id === "climax") {
        notify(
          t("handsFree.climaxDone", {
            count: res.enqueued ?? 0,
            phase: res.phase ?? "recovery",
          }),
          "ok",
        );
      } else {
        notify(t("handsFree.intensitySet", { value: Math.round(res.intensity ?? intensity) }), "ok");
      }
      refreshQueue();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setSignalBusy(null);
    }
  };

  if (error) {
    return (
      <div className="page">
        <div className="alert alert-warn">
          {t("layout.apiUnavailable")} <code>python -m app.main</code>
        </div>
      </div>
    );
  }

  return (
    <div className="page hands-free page--fill">
      <div className="session-toolbar">
        <div className="session-toolbar-text">
          <strong>{t("handsFree.title")}</strong>
          <span className="hint">
            {t("handsFree.subtitle", { intensity: Math.round(intensity) })}
            {snap?.hands_free_favorites_only ? ` · ${t("handsFree.favoritesOnly")}` : ""}
            {snap?.hands_free_last_signal
              ? ` · ${t("handsFree.lastSignal", { signal: snap.hands_free_last_signal })}`
              : ""}
          </span>
        </div>
        <button
          type="button"
          className={`btn ${active ? "btn-warn" : "btn-primary"}`}
          disabled={busy}
          onClick={toggleAuto}
        >
          {active ? t("handsFree.stopAuto") : t("handsFree.startAuto")}
        </button>
      </div>

      <div className="session-workspace hands-free-workspace">
        <SessionRail
          snap={snap}
          queueBlocks={queueDetail?.blocks}
          queueEmptyMessage={
            active ? t("handsFree.queueWaiting") : t("handsFree.queueStart")
          }
        />

        <section className="glass hands-free-signals">
          <span className="section-label">{t("handsFree.signals.title")}</span>
          <p className="hint hands-free-signals-hint">{t("handsFree.signals.hint")}</p>
          <div className="signal-card-grid">
            {signals.map((sig) => (
              <button
                key={sig.id}
                type="button"
                className={`signal-card${sig.urgent ? " signal-card--urgent" : ""}${
                  !active ? " signal-card--disabled" : ""
                }`}
                disabled={!active || signalBusy !== null}
                onClick={() => sendSignal(sig.id)}
              >
                <strong>{sig.title}</strong>
                <span className="hint">{sig.hint}</span>
                {signalBusy === sig.id && (
                  <span className="signal-card-loading">…</span>
                )}
              </button>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}
