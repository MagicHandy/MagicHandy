import { useTranslation } from "react-i18next";
import { PersonaAvatar } from "./PersonaAvatar";

export function TypingIndicator({
  name,
  avatarUrl,
}: {
  name?: string;
  avatarUrl?: string | null;
}) {
  const { t } = useTranslation();
  const displayName = name ?? t("persona.defaultName");

  return (
    <div
      className="bubble-row assistant"
      aria-live="polite"
      aria-label={t("chat.typing", { name: displayName })}
    >
      <PersonaAvatar url={avatarUrl} name={name} size={34} />
      <div className="bubble assistant typing-bubble">
        <span className="typing-dots" aria-hidden>
          <span />
          <span />
          <span />
        </span>
      </div>
    </div>
  );
}
