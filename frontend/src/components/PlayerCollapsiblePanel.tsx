import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

export type PlayerPanelVariant = "preview" | "queue" | "library";

export function PlayerCollapsiblePanel({
  title,
  subtitle,
  open,
  onToggle,
  variant,
  children,
}: {
  title: string;
  subtitle?: string;
  open: boolean;
  onToggle: () => void;
  variant?: PlayerPanelVariant;
  children: ReactNode;
}) {
  const variantClass = variant ? ` player-collapsible--${variant}` : "";
  return (
    <section
      className={`player-collapsible glass${variantClass}${open ? "" : " player-collapsible--collapsed"}`}
    >
      <header className="player-collapsible-head">
        <button
          type="button"
          className="player-collapsible-toggle"
          onClick={onToggle}
          aria-expanded={open}
        >
          <span className="player-collapsible-chevron" aria-hidden>
            {open ? "▾" : "▸"}
          </span>
          <div className="player-collapsible-titles">
            <h2 className="panel-title">{title}</h2>
            {subtitle && <p className="hint player-collapsible-sub">{subtitle}</p>}
          </div>
        </button>
      </header>
      {open && <div className="player-collapsible-body">{children}</div>}
    </section>
  );
}

export type PlayerPanelKey = "preview" | "queue" | "library";

export function PlayerPanelToggles({
  panels,
  onToggle,
}: {
  panels: Record<PlayerPanelKey, boolean>;
  onToggle: (key: PlayerPanelKey) => void;
}) {
  const { t } = useTranslation();
  const panelKeys: PlayerPanelKey[] = ["preview", "queue", "library"];
  return (
    <div className="player-panel-toggles" role="group" aria-label={t("player.panels.visibleAria")}>
      {panelKeys.map((key) => (
        <button
          key={key}
          type="button"
          className={`player-panel-toggle${panels[key] ? " active" : ""}`}
          onClick={() => onToggle(key)}
          aria-pressed={panels[key]}
        >
          {t(`player.panels.${key}`)}
        </button>
      ))}
    </div>
  );
}
