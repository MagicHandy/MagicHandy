import { useTranslation } from "react-i18next";
import type { ChatAutoState, StatusSnapshot } from "../../api/types";
import { PersonaAvatar } from "../PersonaAvatar";
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

function moodLabel(t: (key: string) => string, humor: string, spiceLevel?: string): string {
  if (spiceLevel) {
    const spiceKey = `chatAuto.spiceLevels.${spiceLevel}`;
    const spiceTranslated = t(spiceKey);
    if (spiceTranslated !== spiceKey) {
      return spiceTranslated;
    }
  }
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
  const spiceLevel = state.spice_level;
  const personaName = snap?.persona_name ?? t("persona.defaultName");
  const avatarUrl = snap?.persona_avatar_url ?? null;
  const hasMotion = Boolean(state.motion?.action);

  return (
    <aside className="session-rail chat-auto-rail" aria-label={t("chatAuto.railAria")}>
      <section className="glass chat-auto-hero" aria-label={personaName}>
        <div className="chat-auto-hero-media">
          {avatarUrl ? (
            <img
              src={avatarUrl}
              alt={personaName}
              className="chat-auto-hero-img"
              loading="lazy"
            />
          ) : (
            <div className="chat-auto-hero-fallback" aria-hidden>
              <PersonaAvatar url={null} name={personaName} size={96} />
              <span className="chat-auto-hero-placeholder">{t("chatAuto.imageSoon")}</span>
            </div>
          )}
        </div>
        <div className="chat-auto-hero-caption">
          <strong>{personaName}</strong>
          {autoOn ? (
            <span className="chat-auto-hero-mood">{moodLabel(t, humor, spiceLevel)}</span>
          ) : (
            <span className="hint">{t("chatAuto.idleHint")}</span>
          )}
        </div>
      </section>

      {autoOn ? (
        <section className="glass chat-auto-status">
          {state.llm_busy && (
            <div className="chat-auto-thinking" role="status" aria-live="polite">
              <span className="chat-auto-thinking-dot" aria-hidden />
              {t("chatAuto.thinking")}
            </div>
          )}

          <div className="chat-auto-metrics">
            <StatusBar
              label={t("chatAuto.stamina")}
              value={state.stamina ?? 100}
              variant="stamina"
            />
            <StatusBar
              label={t("chatAuto.humor")}
              value={state.mood_progress ?? 0}
              valueLabel={moodLabel(t, humor, spiceLevel)}
              variant="mood"
            />
          </div>

          <article className="chat-auto-pose-card">
            <span className="chat-auto-pose-label">{t("chatAuto.posicao")}</span>
            <strong className="chat-auto-pose-value">
              {poseLabel(t, state.posicao ?? "handjob")}
            </strong>
          </article>

          <div className="chat-auto-motion-slot">
            {hasMotion ? (
              <MotionChoicePanel motion={state.motion} />
            ) : (
              <div className="chat-auto-motion-idle" aria-hidden>
                <span>{t("chatAuto.motion.idle")}</span>
              </div>
            )}
          </div>

          {state.error && (
            <p className="chat-auto-error" role="alert">
              {state.error}
            </p>
          )}
        </section>
      ) : null}
    </aside>
  );
}
