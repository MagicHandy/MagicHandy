const modalControlSelector = [
  "a[href]",
  "button:not(:disabled)",
  "input:not(:disabled)",
  "select:not(:disabled)",
  "textarea:not(:disabled)",
  "audio[controls]",
  "video[controls]",
  '[tabindex]:not([tabindex="-1"])',
].join(", ");

// Modal focus remains bounded while preserving keyboard access to the global
// safety control, which intentionally sits outside routed content.
export function trapModalTab(event: KeyboardEvent, dialog: HTMLElement): void {
  if (event.key !== "Tab") return;
  const controls = Array.from(dialog.querySelectorAll<HTMLElement>(modalControlSelector));
  const emergencyStop = document.querySelector<HTMLElement>("[data-emergency-stop]");
  if (emergencyStop) controls.push(emergencyStop);
  const unique = Array.from(new Set(controls));
  if (unique.length === 0) return;

  const current = unique.indexOf(document.activeElement as HTMLElement);
  const next = event.shiftKey
    ? unique[(current <= 0 ? unique.length : current) - 1]
    : unique[(current + 1) % unique.length];
  event.preventDefault();
  next.focus();
}
