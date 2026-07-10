import { NavLink } from "react-router-dom";
import type { ComponentType } from "react";
import { useTranslation } from "react-i18next";
import type { StatusSnapshot } from "../api/types";
import {
  IconControl,
  IconHandsFree,
  IconLibrary,
  IconMouse,
  IconPlayer,
  IconSettings,
} from "./icons/NavIcons";
import { PositionVisualizer } from "./PositionVisualizer";

export function Sidebar({
  snap,
  error,
}: {
  snap: StatusSnapshot | null;
  error: string | null;
}) {
  const { t } = useTranslation();

  const navSession = [
    { to: "/", label: t("nav.control"), Icon: IconControl },
    { to: "/freestyle", label: t("nav.freestyle"), Icon: IconHandsFree },
    { to: "/controle-mouse", label: t("nav.mouse"), Icon: IconMouse },
  ] as const;

  const navLibrary = [
    { to: "/biblioteca", label: t("nav.library"), Icon: IconLibrary },
    { to: "/fila", label: t("nav.player"), Icon: IconPlayer },
  ] as const;

  const navSystem = [
    { to: "/config", label: t("nav.settings"), Icon: IconSettings },
  ] as const;

  return (
    <aside className="sidebar sidebar--v12" aria-label={t("nav.mainAria")}>
      <header className="sidebar-header">
        <div className="sidebar-brand">
          <div className="sidebar-brand-mark">
            <img
              className="sidebar-brand-logo"
              src="/logo.jpeg"
              alt="MagicHandy"
              decoding="async"
            />
          </div>
          <div className="sidebar-brand-text">
            <strong className="sidebar-brand-name">MagicHandy</strong>
            <span className="sidebar-brand-tag">{t("nav.group.session")} · Local</span>
          </div>
        </div>
      </header>

      <nav className="sidebar-nav">
        <NavGroup label={t("nav.group.session")} items={navSession} snap={snap} t={t} />
        <NavGroup label={t("nav.group.library")} items={navLibrary} snap={snap} t={t} />
        <NavGroup label={t("nav.group.system")} items={navSystem} snap={snap} t={t} />
      </nav>

      <div className="sidebar-motion">
        <PositionVisualizer variant="sidebar" />
      </div>

      {error && (
        <div className="sidebar-offline-card">
          <p className="sidebar-offline-title">{t("layout.offline.title")}</p>
          <p className="hint">{t("layout.offline.hint")}</p>
          <code className="launch-cmd">Iniciar-MagicHandy.bat</code>
        </div>
      )}
    </aside>
  );
}

function NavGroup({
  label,
  items,
  snap,
  t,
}: {
  label: string;
  items: readonly {
    to: string;
    label: string;
    Icon: ComponentType<{ className?: string }>;
  }[];
  snap: StatusSnapshot | null;
  t: (key: string, opts?: Record<string, unknown>) => string;
}) {
  return (
    <div className="nav-group">
      <span className="nav-group-label">{label}</span>
      {items.map((item) => {
        const queueCount = snap?.manual_queue_count ?? 0;
        const queuePlaying = snap?.manual_queue_playing;
        const showBadge = item.to === "/fila" && queueCount > 0;
        const Icon = item.Icon;
        return (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === "/"}
            className={({ isActive }) =>
              `nav-link nav-link--pro${isActive ? " active" : ""}${queuePlaying && item.to === "/fila" ? " nav-link--playing" : ""}`
            }
            title={item.label}
          >
            <span className="nav-link-icon-wrap">
              <Icon className="nav-link-icon" />
            </span>
            <span className="nav-link-label">{item.label}</span>
            {showBadge && (
              <span
                className={`nav-badge${queuePlaying ? " nav-badge--live" : ""}`}
                title={
                  queuePlaying
                    ? t("nav.queuePlaying", { name: snap?.manual_queue_name ?? t("nav.queue") })
                    : t("nav.queueCount", { count: queueCount })
                }
              >
                {queuePlaying ? "▶" : queueCount}
              </span>
            )}
          </NavLink>
        );
      })}
    </div>
  );
}
