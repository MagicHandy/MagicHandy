import { Children, isValidElement, type ReactNode } from "react";

// Every setup step is built from the same two pieces: a section card with a
// header row, and a choice list whose selected option's details sit in a
// panel under the list, inside the same border.

export function SetupSection({ id, title, aside, children }: { id: string; title: string; aside?: string; children: ReactNode }) {
  return <section className="setup-section" aria-labelledby={id}>
    <h2 id={id}>{title}{aside && <span className="hint-inline">{aside}</span>}</h2>
    {children}
  </section>;
}

export function SetupChoiceGroup({ label, labelledBy, panel, className, children }: { label?: string; labelledBy?: string; panel?: ReactNode; className?: string; children: ReactNode }) {
  // The selected option's details open directly under it, inside the list's
  // border, so what an option needs reads as part of that option.
  // With no option chosen (a saved setup none of them describes), the panel
  // leads the list.
  const box = panel && <div key="setup-choice-panel" className="setup-choice-panel form-rows">{panel}</div>;
  const options = Children.toArray(children);
  const chosen = options.some((child) => isValidElement<{ selected?: boolean }>(child) && child.props.selected);
  const items = chosen ? options.flatMap((child): ReactNode[] => isValidElement<{ selected?: boolean }>(child) && child.props.selected ? [child, box] : [child]) : [box, ...options];
  return <div className={className ? `setup-choices ${className}` : "setup-choices"} role="radiogroup" aria-label={label} aria-labelledby={labelledBy}>{items}</div>;
}

// A field row: the label and its explanation on the left, the control
// (given the matching id and aria-describedby `${id}-hint`) on the right, or
// under the label when `stack` is set for wide values such as URLs.
export function FieldRow({ id, label, hint, stack, children }: { id: string; label: string; hint?: ReactNode; stack?: boolean; children: ReactNode }) {
  return <div className={stack ? "form-row form-row-stack" : "form-row"}>
    <span className="form-row-label"><label htmlFor={id}><strong>{label}</strong></label>{hint && <small id={`${id}-hint`}>{hint}</small>}</span>
    {children}
  </div>;
}
