import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { t, translateKnown } from "../i18n";
import { ArrowLeftIcon, ArrowRightIcon } from "../shell/icons";

export function SettingsNavigationLabel({ label, compact }: { label: string; compact?: string }) {
  if (!compact) return <>{translateKnown(label)}</>;
  return <><span className="settings-nav-full">{translateKnown(label)}</span><span className="settings-nav-compact" aria-hidden="true">{translateKnown(compact)}</span></>;
}

// Both settings levels keep their links and URLs. Narrow layouts scroll only
// this strip, so revealing the selected page never moves the content vertically.
export function SettingsNavigation({ label, current, className, children }: {
  label: string; current: string; className: string; children: ReactNode;
}) {
  const root = useRef<HTMLElement>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const [overflow, setOverflow] = useState({ visible: false, previous: false, next: false });
  const measure = useCallback(() => {
    const strip = viewport.current, nav = root.current;
    if (!strip || !nav) return;
    // Use the full available width to avoid arrows keeping themselves visible
    // when all links would fit after the arrows are removed.
    const next = {
      visible: nav.clientWidth > 0 && strip.scrollWidth > nav.clientWidth + 1,
      previous: strip.scrollLeft > 1,
      next: strip.scrollLeft + strip.clientWidth < strip.scrollWidth - 1,
    };
    setOverflow(old => old.visible === next.visible && old.previous === next.previous && old.next === next.next ? old : next);
  }, []);
  const reveal = useCallback(() => {
    const strip = viewport.current, active = strip?.querySelector<HTMLElement>('a[aria-current="page"]');
    if (!strip || !active || strip.scrollWidth <= strip.clientWidth) return;
    const bounds = strip.getBoundingClientRect(), item = active.getBoundingClientRect();
    if (item.left < bounds.left) strip.scrollLeft += item.left - bounds.left;
    else if (item.right > bounds.right) strip.scrollLeft += item.right - bounds.right;
  }, []);

  useLayoutEffect(() => { reveal(); measure(); }, [current, label, overflow.visible, measure, reveal]);
  useLayoutEffect(() => {
    if (!root.current || !viewport.current || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => { reveal(); measure(); });
    observer.observe(root.current);
    observer.observe(viewport.current);
    return () => observer.disconnect();
  }, [measure, reveal]);

  const scroll = (direction: number) => {
    const strip = viewport.current;
    if (!strip) return;
    strip.scrollLeft = Math.max(0, Math.min(strip.scrollWidth - strip.clientWidth, strip.scrollLeft + direction * strip.clientWidth * .75));
    measure();
  };
  return <nav ref={root} className={`settings-navigation ${className}`} aria-label={label}>
    <button className="settings-nav-scroll" type="button" hidden={!overflow.visible} disabled={!overflow.previous} aria-label={t("Scroll sections left")} onClick={() => scroll(-1)}><ArrowLeftIcon size={16} /></button>
    <div ref={viewport} className="settings-nav-scrollport" onScroll={measure}>{children}</div>
    <button className="settings-nav-scroll" type="button" hidden={!overflow.visible} disabled={!overflow.next} aria-label={t("Scroll sections right")} onClick={() => scroll(1)}><ArrowRightIcon size={16} /></button>
  </nav>;
}
