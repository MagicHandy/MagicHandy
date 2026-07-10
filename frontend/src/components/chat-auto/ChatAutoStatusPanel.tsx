import { useTranslation } from "react-i18next";
import type { ChatAutoState, StatusSnapshot } from "../../api/types";
import { MotionChoicePanel } from "./MotionChoicePanel";
import { StatusBar } from "./StatusBar";

function resolveAutoState(snap: StatusSnapshot | null): ChatAutoState {
  if (snap?.chat_auto) return snap.chat_auto;
  return {
    stamina: 100,
    humor: "desejando",
    mood_progress: 0,
    posicao: "handjob",
    active: snap?.operation_mode === "auto",
  };
}

function moodLabel(t: (key: string) => string, humor: string): string {
  const key = `chatAuto.moods.${humor}`;
  const translated = t(key);
  return translated === key ? humor : translated;
}

function poseLabel(t: (key: string) => string, pose: string): string {
  const key = `chatAuto.poses.${pose}`;
  const translated = t(key);
  return translated === key ? pose : translated;
}

export function ChatAutoStatusPanel({ snap }: { snap: StatusSnapshot | null }) {
  const { t } = useTranslation();
  const state = resolveAutoState(snap);
  const autoOn = snap?.operation_mode === "auto" || Boolean(state.active);
  const humor = state.humor ?? "desejando";

  return (
    <aside className="session-rail chat-auto-rail" aria-label={t("chatAuto.railAria")}>
      <section className="glass chat-auto-image-slot" aria-hidden>
        <span className="chat-auto-image-placeholder">{t("chatAuto.imageSoon")}</span>
      </section>

      {autoOn ? (
        <section className="panel chat-auto-status">
          <StatusBar
            label={t("chatAuto.stamina")}
            value={state.stamina ?? 100}
            variant="stamina"
          />
          <StatusBar
            label={t("chatAuto.humor")}
            value={state.mood_progress ?? 0}
            valueLabel={moodLabel(t, humor)}
            variant="mood"
          />
          <div className="chat-auto-pose-row">
            <span className="label">{t("chatAuto.posicao")}</span>
            <strong>{poseLabel(t, state.posicao ?? "handjob")}</strong>
          </div>
          {state.llm_busy && <p className="hint">{t("chatAuto.thinking")}</p>}
          <MotionChoicePanel motion={state.motion} />
          {state.error && <p className="hint autodom-error">{state.error}</p>}
          {state.last_reply && (
            <p className="hint chat-auto-live-line">{state.last_reply}</p>
          )}
        </section>
      ) : (
        <section className="panel chat-auto-idle">
          <p className="hint">{t("chatAuto.idleHint")}</p>
        </section>
      )}
    </aside>
  );
}
