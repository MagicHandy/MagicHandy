import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { LanguageSelector } from "../components/LanguageSelector";
import { PageHeader } from "../components/PageHeader";
import { PromptSetEditor } from "../components/PromptSetEditor";
import { useStatus } from "../contexts/StatusContext";
import {
  getConfigNav,
  type ConfigSectionId,
  isConfigSectionId,
} from "../config/configNav";
import { DiagnosticsPanel } from "./Diagnostics";
import { PersonasPanel } from "./Personas";
import { SessionsPanel } from "./Sessions";
import { SettingsPanel, SettingsRawPanel } from "./Settings";

const SETTINGS_SECTIONS: SettingsSection[] = [
  "session",
  "motion",
  "connections",
  "voice",
  "logs",
];

type SettingsSection = Extract<
  ConfigSectionId,
  "session" | "motion" | "connections" | "voice" | "logs"
>;

function isSettingsSection(id: ConfigSectionId): id is SettingsSection {
  return SETTINGS_SECTIONS.includes(id as SettingsSection);
}

export function ConfigHub() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const [section, setSection] = useState<ConfigSectionId>("personas");
  const [filter, setFilter] = useState("");

  const configNav = useMemo(() => getConfigNav(t), [t]);
  const { readOnly } = useStatus();

  useEffect(() => {
    const hash = location.hash.replace("#", "");
    if (isConfigSectionId(hash)) setSection(hash);
  }, [location.hash]);

  const selectSection = (id: ConfigSectionId) => {
    setSection(id);
    navigate({ hash: id }, { replace: true });
  };

  const filteredNav = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return configNav;
    return configNav.filter(
      (item) =>
        item.label.toLowerCase().includes(q) ||
        item.hint.toLowerCase().includes(q) ||
        item.group.toLowerCase().includes(q),
    );
  }, [filter, configNav]);

  const groups = useMemo(() => {
    const map = new Map<string, typeof configNav>();
    for (const item of filteredNav) {
      const list = map.get(item.group) ?? [];
      list.push(item);
      map.set(item.group, list);
    }
    return [...map.entries()];
  }, [filteredNav]);

  const active = configNav.find((item) => item.id === section);

  return (
    <div className="page page--fill config-page">
      <PageHeader
        title={t("config.title")}
        intro={t("config.intro")}
        compact
      />
      <div className="config-workspace">
        <aside className="glass config-nav" aria-label={t("config.navAria")}>
          <div className="config-nav-head">
            <span className="section-label">{t("config.title")}</span>
            <input
              type="search"
              className="config-nav-search"
              placeholder={t("config.searchPlaceholder")}
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              aria-label={t("config.searchAria")}
            />
          </div>
          <nav className="config-nav-list">
            {groups.map(([group, items]) => (
              <div key={group} className="config-nav-group">
                <span className="config-nav-group-label">{group}</span>
                {items.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    className={`config-nav-item${section === item.id ? " active" : ""}`}
                    onClick={() => selectSection(item.id)}
                  >
                    <span className="config-nav-item-label">{item.label}</span>
                    <span className="config-nav-item-hint">{item.hint}</span>
                  </button>
                ))}
              </div>
            ))}
            {groups.length === 0 && (
              <p className="hint config-nav-empty">{t("config.noSections")}</p>
            )}
          </nav>
        </aside>

        <div className="config-main">
          {active && (
            <header className="config-section-head">
              <h2 className="config-section-title">{active.label}</h2>
              <p className="hint">{active.hint}</p>
            </header>
          )}

          <div className="config-section-body page-scroll">
            {section === "language" && (
              <section className="glass settings-card">
                <LanguageSelector />
              </section>
            )}
            {section === "personas" && <PersonasPanel />}
            {isSettingsSection(section) && <SettingsPanel section={section} />}
            {section === "advanced" && (
              <>
                <PromptSetEditor locked={readOnly} />
                <SettingsRawPanel />
              </>
            )}
            {section === "sessions" && <SessionsPanel />}
            {section === "diagnostics" && <DiagnosticsPanel />}
          </div>
        </div>
      </div>
    </div>
  );
}
