import { t, translateKnown } from "../i18n";
// Permanent left navigation rail: product identity, page links, and the pinned
// Stop footer. The rail is present on every route (docs/ui-navigation-redesign.md).
import { useAppState, useHashRoute } from "../state/app-state";
import { ChatIcon, LibraryIcon, ModesIcon, PersonaIcon, SettingsIcon, VideoIcon } from "./icons";
import { StopButton } from "./StopButton";

// Personas sits second, directly under Chat: a persona is chat furniture, closer
// to the conversation than the content libraries are (docs/persona-page.md §5.1).
const LINKS = [
  { base: "chat", href: "#/chat", label: "Chat", Icon: ChatIcon },
  { base: "personas", href: "#/personas", label: "Personas", Icon: PersonaIcon },
  { base: "modes", href: "#/modes", label: "Preset modes", Icon: ModesIcon },
  { base: "library", href: "#/library", label: "Pattern library", Icon: LibraryIcon },
  { base: "videos", href: "#/videos", label: "Videos", Icon: VideoIcon },
  { base: "settings", href: "#/settings", label: "Settings", Icon: SettingsIcon },
] as const;

export function routeBase(hash: string): string {
  const candidate = hash.replace(/^#\/?/, "").split("/")[0] || "chat";
  if (candidate === "setup") return "setup";
  return LINKS.some((link) => link.base === candidate) ? candidate : "chat";
}

export function NavRail({ authenticationLocked = false }: { authenticationLocked?: boolean }) {
  const active = routeBase(useHashRoute());
  const { state } = useAppState();
  const owner = state?.settings?.device?.hsp_dispatch_owner ?? "cloud";
  const ownerLabel = {
    cloud_rest: "Cloud REST",
    browser_bluetooth: "Browser Bluetooth",
    intiface: "Intiface Central",
  }[owner] ?? owner.replace(/_/g, " ");
  return (
    <nav className="nav-rail" aria-label={t("Primary navigation")}>
      <div className="nav-brand">
        <span className="nav-brand-mark" aria-hidden="true">M</span>
        <span className="nav-brand-copy">
          <span className="nav-brand-name">{t("MagicHandy")}</span>
          <span className="nav-brand-context">{authenticationLocked ? t("sign-in required") : t("local / {owner}", { owner: translateKnown(ownerLabel) })}</span>
        </span>
      </div>
      {!authenticationLocked && <div className="nav-links">
        {LINKS.map((l) => (
          <a key={l.base} className="nav-link" href={l.href} aria-label={translateKnown(l.label)} aria-current={active === l.base ? "page" : undefined}>
            <span className="icon"><l.Icon /></span>
            <span className="label">{translateKnown(l.label)}</span>
          </a>
        ))}
      </div>}
      <div className="nav-spacer" />
      <StopButton className="nav-stop" />
    </nav>
  );
}
