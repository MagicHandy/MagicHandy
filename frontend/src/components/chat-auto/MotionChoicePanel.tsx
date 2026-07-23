import { useTranslation } from "react-i18next";
import type { ChatAutoMotion } from "../../api/types";
import { PositionSparkline } from "../PositionSparkline";
import { IntensityFlame } from "./IntensityFlame";
import { SpeedGauge } from "./SpeedGauge";
import { StatusBar } from "./StatusBar";

function labelFor(
  t: (key: string) => string,
  group: "regions" | "curves",
  value: string,
): string {
  const key = `chatAuto.${group}.${value}`;
  const translated = t(key);
  return translated === key ? value.replace(/_/g, " ") : translated;
}

function actionLabel(t: (key: string) => string, action: string): string {
  const key = `chatAuto.motion.actions.${action}`;
  const translated = t(key);
  return translated === key ? action : translated;
}

export function MotionChoicePanel({ motion }: { motion?: ChatAutoMotion }) {
  const { t } = useTranslation();
  if (!motion?.action) return null;

  const delayMs = motion.atraso_ms ?? 0;
  const delayPct = Math.max(0, Math.min(100, 100 - delayMs / 3));

  return (
    <section className="motion-choice-panel" aria-label={t("chatAuto.motion.title")}>
      <header className="motion-choice-head">
        <span className="motion-choice-title">{t("chatAuto.motion.title")}</span>
        <span className="motion-choice-action">{actionLabel(t, motion.action)}</span>
      </header>

      <div className="motion-choice-meta">
        <span className="motion-chip">{labelFor(t, "regions", motion.regiao ?? "meio_cabeca")}</span>
        <span className="motion-chip motion-chip--curve">
          {labelFor(t, "curves", motion.tipo_batida ?? "fluido")}
        </span>
      </div>

      <PositionSparkline />

      <div className="motion-choice-gauges">
        <SpeedGauge value={motion.velocidade ?? 0} label={t("chatAuto.motion.speed")} />
        <IntensityFlame value={motion.intensidade ?? 0} label={t("chatAuto.motion.intensity")} />
      </div>

      <StatusBar
        label={t("chatAuto.motion.delay")}
        value={delayPct}
        valueLabel={`${delayMs} ms`}
        variant="delay"
      />
    </section>
  );
}
