import { useEffect, useRef } from "react";

// Every route sets its heading and moves focus there on entry, so keyboard and
// screen-reader users land in the new page (docs/ui-navigation-redesign.md).
export function WorkspaceHead({ title, lede }: { title: string; lede?: string }) {
  const ref = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, [title]);
  return (
    <header className="workspace-head">
      <h1 ref={ref} tabIndex={-1}>{title}</h1>
      {lede && <p className="lede">{lede}</p>}
    </header>
  );
}
