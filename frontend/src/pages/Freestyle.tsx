import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import { SessionRail } from "../components/SessionRail";
import { PageHeader } from "../components/PageHeader";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";

const STYLES = ["gentle", "balanced", "intense"] as const;

export function Freestyle() {
  const { t } = useTranslation();
  const { snap, error, refresh } = useStatus();
  const { notify } = useToast();
  const [busy, setBusy] = useState(false);
  const [modes, setModes] = useState<Awaited<ReturnType<typeof api.getModes>> | null>(null);
  const [style, setStyle] = useState("balanced");

  const loadModes = useCallback(async () => {
    try {
      const status = await api.getModes();
      setModes(status);
    } catch {
      setModes(null);
    }
  }, []);

  useEffect(() => {
    void loadModes();
    const id = setInterval(() => void loadModes(), 2000);
    return () => clearInterval(id);
  }, [loadModes, snap?.playback_active]);

  useEffect(() => {
    void api.getSettings().then((s) => {
      const m = s.motion as { style?: string } | undefined;
      if (m?.style) setStyle(m.style);
    });
  }, []);

  const freestyleActive =
    modes?.active === true ||
    modes?.running === true ||
    modes?.mode === "freestyle" ||
    modes?.active_mode === "freestyle";

  const toggleFreestyle = async () => {
    if (busy) return;
    setBusy(true);
    try {
      if (freestyleActive) {
        await api.stopMode();
        notify(t("freestyle.stopped"), "info");
      } else {
        await api.startMode("freestyle");
        notify(t("freestyle.started"), "ok");
      }
      await loadModes();
      await refresh();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const setStyleValue = async (next: string) => {
    try {
      await api.applyQuick({ style: next });
      setStyle(next);
      await refresh();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <div className="freestyle-workspace">
      <SessionRail snap={snap} queueEmptyMessage={t("session.queueEmpty")} />
      <div className="workspace-main">
        <PageHeader
          title={t("freestyle.title")}
          subtitle={t("freestyle.subtitle")}
          intro={t("freestyle.intro")}
          compact
        />

        <section className="panel">
          <h2 className="section-title">{t("freestyle.panelTitle")}</h2>
          <p className="hint">{t("freestyle.panelHint")}</p>
        <div className="row-actions">
          <button
            type="button"
            className={freestyleActive ? "btn btn-secondary" : "btn btn-start"}
            onClick={() => void toggleFreestyle()}
            disabled={Boolean(error) || busy}
          >
            {freestyleActive ? t("freestyle.stop") : t("freestyle.start")}
          </button>
        </div>

        <div className="field" style={{ marginTop: "1rem" }}>
          <span className="label">{t("freestyle.styleLabel")}</span>
          <div className="segmented" role="group" aria-label={t("freestyle.styleLabel")}>
            {STYLES.map((s) => (
              <button
                key={s}
                type="button"
                aria-pressed={style === s}
                disabled={Boolean(error) || busy}
                onClick={() => void setStyleValue(s)}
              >
                {t(`freestyle.styles.${s}`)}
              </button>
            ))}
          </div>
        </div>
      </section>
      </div>
    </div>
  );
}
