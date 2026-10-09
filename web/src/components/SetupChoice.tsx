export function SetupChoice({ selected, title, detail, badge, disabled, onSelect }: { selected: boolean; title: string; detail: string; badge?: string; disabled?: boolean; onSelect: () => void }) {
  return <label className="setup-choice" data-selected={selected} data-disabled={disabled || undefined}>
    <input type="radio" checked={selected} disabled={disabled} onChange={onSelect} />
    <span className="setup-choice-copy"><strong>{title}{badge && <span className="setup-badge">{badge}</span>}</strong><small>{detail}</small></span>
  </label>;
}
