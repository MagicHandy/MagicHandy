import { useEffect, useRef } from "react";

// Every route sets its heading and moves focus there on entry, so keyboard and
// screen-reader users land in the new page (docs/ui-navigation-redesign.md).
// `wide` matches the header to the wide two-column (.split) content width so
// the title left-aligns with the content below it instead of sitting in the
// narrower default column.
// `hidden` keeps the heading for screen readers and focus while a page such
// as the watch page gives its space to content.
export function WorkspaceHead({ title, lede, wide, hidden }: { title: string; lede?: string; wide?: boolean; hidden?: boolean }) {
  const ref = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    const pageTitle = `${title} | MagicHandy`;
    document.title = pageTitle;
    ref.current?.focus();
    return () => {
      if (document.title === pageTitle) document.title = "MagicHandy";
    };
  }, [title]);
  return (
    <header className={hidden ? "workspace-head visually-hidden" : "workspace-head"} data-wide={(wide && !hidden) || undefined}>
      <h1 ref={ref} tabIndex={-1}>{title}</h1>
      {lede && <p className="lede">{lede}</p>}
    </header>
  );
}
