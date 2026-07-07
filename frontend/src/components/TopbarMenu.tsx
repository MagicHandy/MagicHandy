import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

interface TopbarMenuProps {
  label: string;
  connected?: boolean;
  detail?: string | null;
  badge?: ReactNode;
  align?: "left" | "right";
  children: ReactNode;
}

export function TopbarMenu({
  label,
  connected,
  detail,
  badge,
  align = "left",
  children,
}: TopbarMenuProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const panelId = useId();

  useEffect(() => {
    if (!open) return;
    const onPointer = (e: MouseEvent) => {
      if (rootRef.current?.contains(e.target as Node)) return;
      setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const statusKnown = connected !== undefined;

  return (
    <div className="topbar-menu" ref={rootRef}>
      <button
        type="button"
        className={`conn-pill menu-trigger${open ? " open" : ""}${
          statusKnown ? (connected ? " on" : " off") : ""
        }`}
        aria-expanded={open}
        aria-controls={panelId}
        title={detail ?? undefined}
        onClick={() => setOpen((v) => !v)}
      >
        {statusKnown && <span className="conn-dot" />}
        <span className="conn-label">{label}</span>
        {badge}
        {statusKnown && (
          <span className="conn-state">{connected ? t("common.ok") : t("common.off")}</span>
        )}
        <span className="menu-chevron" aria-hidden>
          ▾
        </span>
      </button>
      {open && (
        <div
          id={panelId}
          className={`topbar-menu-panel align-${align}`}
          role="menu"
        >
          {children}
        </div>
      )}
    </div>
  );
}
