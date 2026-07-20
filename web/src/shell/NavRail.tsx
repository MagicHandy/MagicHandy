// Permanent left navigation rail: profile lockup, page links, and the pinned
// Stop footer. The rail is present on every route (docs/ui-navigation-redesign.md).
import { useAppState, useHashRoute } from "../state/app-state";
import { ChatIcon, LibraryIcon, ModesIcon, SettingsIcon, VideoIcon } from "./icons";
import { StopButton } from "./StopButton";

const LINKS = [
  { base: "chat", href: "#/chat", label: "Chat", Icon: ChatIcon },
  { base: "modes", href: "#/modes", label: "Preset modes", Icon: ModesIcon },
  { base: "library", href: "#/library", label: "Pattern library", Icon: LibraryIcon },
  { base: "videos", href: "#/videos", label: "Videos", Icon: VideoIcon },
  { base: "settings", href: "#/settings", label: "Settings", Icon: SettingsIcon },
] as const;

export function routeBase(hash: string): string {
  const candidate = hash.replace(/^#\/?/, "").split("/")[0] || "chat";
  return LINKS.some((link) => link.base === candidate) ? candidate : "chat";
}

export function NavRail() {
  const active = routeBase(useHashRoute());
  const { state } = useAppState();
  const owner = state?.settings?.device?.hsp_dispatch_owner ?? "cloud";
  const ownerLabel = owner.replace(/_/g, " ");

  return (
    <nav className="nav-rail" aria-label="Primary navigation">
      <a className="nav-profile" href="#/settings">
        <span className="nav-avatar" aria-hidden="true">M</span>
        <span className="nav-identity">
          <span className="name">MagicHandy</span>
          <span className="sub">local / {ownerLabel}</span>
        </span>
      </a>
      <div className="nav-divider" aria-hidden="true" />
      <div className="nav-links">
        {LINKS.map((l) => (
          <a key={l.base} className="nav-link" href={l.href} aria-label={l.label} aria-current={active === l.base ? "page" : undefined}>
            <span className="icon"><l.Icon /></span>
            <span className="label">{l.label}</span>
          </a>
        ))}
      </div>
      <div className="nav-spacer" />
      <StopButton className="nav-stop" />
    </nav>
  );
}
