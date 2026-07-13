import { useEffect, useRef } from "react";

// Every route sets its heading and moves focus there on entry, so keyboard and
// screen-reader users land in the new page (docs/ui-navigation-redesign.md).
// `wide` matches the header to the wide two-column (.split) content width so
// the title left-aligns with the content below it instead of sitting in the
// narrower default column.
export function WorkspaceHead({ title, lede, wide }: { title: string; lede?: string; wide?: boolean }) {
  const ref = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, [title]);
  return (
    <header className="workspace-head" data-wide={wide || undefined}>
      <h1 ref={ref} tabIndex={-1}>{title}</h1>
      {lede && <p className="lede">{lede}</p>}
    </header>
  );
}
