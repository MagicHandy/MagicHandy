import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

type MouseZone = "left" | "right" | "scroll" | "left-hold";

function KeyCap({ children }: { children: ReactNode }) {
  return <kbd className="mc-gesture-key">{children}</kbd>;
}

function MouseIcon({ zone }: { zone: MouseZone }) {
  return (
    <span className="mc-gesture-mouse" aria-hidden>
      <svg viewBox="0 0 28 40" className="mc-gesture-mouse-svg">
        <path
          d="M14 2C8.5 2 4 6.2 4 12v16c0 5.8 4.5 10 10 10s10-4.2 10-10V12C24 6.2 19.5 2 14 2z"
          fill="rgba(15,18,32,0.95)"
          stroke="rgba(148,163,184,0.45)"
          strokeWidth="1.2"
        />
        <line x1="14" y1="5" x2="14" y2="14" stroke="rgba(148,163,184,0.35)" strokeWidth="1" />
        <rect
          x="6"
          y="5"
          width="8"
          height="11"
          rx="3"
          className={`mc-gesture-mouse-zone mc-gesture-mouse-zone--left${zone === "left" || zone === "left-hold" ? " is-active" : ""}`}
        />
        <rect
          x="14"
          y="5"
          width="8"
          height="11"
          rx="3"
          className={`mc-gesture-mouse-zone mc-gesture-mouse-zone--right${zone === "right" ? " is-active" : ""}`}
        />
        <rect
          x="11.5"
          y="7"
          width="5"
          height="5"
          rx="1.5"
          className={`mc-gesture-mouse-zone mc-gesture-mouse-zone--scroll${zone === "scroll" ? " is-active" : ""}`}
        />
        {zone === "left-hold" && (
          <circle cx="10" cy="10.5" r="2.2" className="mc-gesture-mouse-hold-dot" />
        )}
      </svg>
    </span>
  );
}

function GestureRow({
  keys,
  label,
}: {
  keys: ReactNode;
  label: string;
}) {
  return (
    <li className="mc-gesture-row">
      <div className="mc-gesture-keys">{keys}</div>
      <span className="mc-gesture-desc">{label}</span>
    </li>
  );
}

export function MouseControlGestureHints({ dimmed = false }: { dimmed?: boolean }) {
  const { t } = useTranslation();

  return (
    <ul className={`mc-gesture-list${dimmed ? " mc-gesture-list--dimmed" : ""}`}>
      <GestureRow
        keys={
          <>
            <KeyCap>Ctrl</KeyCap>
            <span className="mc-gesture-plus">+</span>
            <MouseIcon zone="scroll" />
          </>
        }
        label={t("mouse.gestures.record")}
      />
      <GestureRow
        keys={
          <>
            <MouseIcon zone="left-hold" />
            <span className="mc-gesture-hold">{t("mouse.gestures.hold")}</span>
          </>
        }
        label={t("mouse.gestures.turbo")}
      />
      <GestureRow
        keys={
          <>
            <KeyCap>Ctrl</KeyCap>
            <span className="mc-gesture-plus">+</span>
            <MouseIcon zone="left" />
          </>
        }
        label={t("mouse.gestures.lessSmooth")}
      />
      <GestureRow
        keys={
          <>
            <KeyCap>Ctrl</KeyCap>
            <span className="mc-gesture-plus">+</span>
            <MouseIcon zone="right" />
          </>
        }
        label={t("mouse.gestures.moreSmooth")}
      />
    </ul>
  );
}
