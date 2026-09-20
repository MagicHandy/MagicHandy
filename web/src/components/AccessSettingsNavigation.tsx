import { t, translateKnown } from "../i18n";
import { SettingsNavigation, SettingsNavigationLabel } from "./SettingsNavigation";

const personal = [
  { id: "profile", label: "Your profile", compact: "Profile" },
  { id: "security", label: "Security" },
  { id: "sessions", label: "Sessions" },
] as const;
const installation = [
  { id: "accounts", label: "Accounts & permissions", compact: "Accounts" },
  { id: "network", label: "Remote access" },
  { id: "history", label: "Access history", compact: "History" },
] as const;
export type AccessSection = (typeof personal)[number]["id"] | (typeof installation)[number]["id"];

export function resolveAccessSection(requested: string, administrator: boolean): AccessSection {
  return [...personal, ...(administrator ? installation : [])].find(item => item.id === requested)?.id || "profile";
}

export function AccessSettingsNavigation({ section, administrator }: { section: AccessSection; administrator: boolean }) {
  const links = (items: ReadonlyArray<{ id: AccessSection; label: string; compact?: string }>) => items.map(item =>
    <li key={item.id}><a href={`#/settings/access/${item.id}`} aria-label={translateKnown(item.label)} title={translateKnown(item.label)} aria-current={section === item.id ? "page" : undefined}><SettingsNavigationLabel label={item.label} compact={item.compact} /></a></li>);
  return <SettingsNavigation className="access-nav settings-sidebar" label={t("Access sections")} current={section}>
    <div className="settings-sidebar-group"><p className="settings-sidebar-label" id="access-personal-label">{t("Your account")}</p><ul aria-labelledby="access-personal-label">{links(personal)}</ul></div>
    {administrator && <div className="settings-sidebar-group"><p className="settings-sidebar-label" id="access-installation-label">{t("This installation")}</p><ul aria-labelledby="access-installation-label">{links(installation)}</ul></div>}
  </SettingsNavigation>;
}
