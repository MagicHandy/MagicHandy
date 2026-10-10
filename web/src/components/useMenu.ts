import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";

// The shared menu-button behavior (WAI-ARIA menu button pattern). Opening moves
// focus to the checked item, or else the first enabled one (the last one when
// opened with ArrowUp). ArrowUp/ArrowDown, Home and End move focus between
// enabled items, and a printable key jumps to the next item that starts with
// it. Choosing an item, Tab, Escape or a press outside closes the menu; every
// close except an outside press returns focus to the opener, which Tab then
// moves on from. Escape is also Emergency Stop's global shortcut (StopButton
// listens on window), so the menu closes on it without ever stopping it.

export type MenuFocus = "selected" | "first" | "last";

interface OpenMenu<Value> {
  opener: HTMLElement;
  value: Value;
  focus: MenuFocus;
}

const itemSelector = '[role="menuitem"], [role="menuitemradio"], [role="menuitemcheckbox"]';

function enabledItems(menu: HTMLElement | null): HTMLElement[] {
  if (!menu) return [];
  return Array.from(menu.querySelectorAll<HTMLElement>(itemSelector))
    .filter((item) => !item.matches(":disabled") && item.getAttribute("aria-disabled") !== "true");
}

// Roving focus: the focused item is the menu's one tab stop.
function focusItem(items: HTMLElement[], index: number) {
  const target = items[index];
  if (!target) return;
  for (const item of items) item.tabIndex = item === target ? 0 : -1;
  target.focus();
}

// The visible label, without decorative (aria-hidden) monograms and icons.
function itemLabel(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? "";
  if (node instanceof Element && node.getAttribute("aria-hidden") === "true") return "";
  return Array.from(node.childNodes, itemLabel).join("");
}

export function useMenu<Value = undefined>({ disabled = false }: { disabled?: boolean } = {}) {
  const [state, setState] = useState<OpenMenu<Value> | null>(null);
  const current = useRef(state);
  current.current = state;
  const menuRef = useRef<HTMLDivElement>(null);
  const id = useId();

  const show = useCallback((opener: HTMLElement, value: Value, focus: MenuFocus = "selected") => {
    setState({ opener, value, focus });
  }, []);

  const close = useCallback((restoreFocus = true) => {
    const open = current.current;
    if (!open) return;
    current.current = null;
    if (restoreFocus && open.opener.isConnected) open.opener.focus();
    setState(null);
  }, []);

  useLayoutEffect(() => {
    if (!state) return;
    const items = enabledItems(menuRef.current);
    if (items.length === 0) {
      menuRef.current?.focus();
      return;
    }
    const checked = state.focus === "selected" ? items.findIndex((item) => item.getAttribute("aria-checked") === "true") : -1;
    focusItem(items, state.focus === "last" ? items.length - 1 : Math.max(0, checked));
  }, [state]);

  useEffect(() => {
    if (!state) return;
    const pressOutside = (event: MouseEvent) => {
      const target = event.target as Node;
      if (menuRef.current?.contains(target) || state.opener.contains(target)) return;
      close(false);
    };
    // Close only: this keypress must still reach Emergency Stop.
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") close(true);
    };
    window.addEventListener("mousedown", pressOutside);
    window.addEventListener("keydown", escape);
    return () => {
      window.removeEventListener("mousedown", pressOutside);
      window.removeEventListener("keydown", escape);
    };
  }, [state, close]);

  // A control that becomes unavailable takes its menu with it.
  useEffect(() => {
    if (disabled) close(false);
  }, [disabled, close]);

  const onMenuKeyDown = useCallback((event: ReactKeyboardEvent<HTMLElement>) => {
    const items = enabledItems(menuRef.current);
    const at = items.indexOf(document.activeElement as HTMLElement);
    let next: number;
    switch (event.key) {
      case "ArrowDown": next = (at + 1) % items.length; break;
      case "ArrowUp": next = at <= 0 ? items.length - 1 : at - 1; break;
      case "Home": next = 0; break;
      case "End": next = items.length - 1; break;
      case "Tab": close(true); return;
      default: {
        if (event.key.length !== 1 || !event.key.trim() || event.altKey || event.ctrlKey || event.metaKey) return;
        const key = event.key.toLocaleLowerCase();
        const ordered = [...items.slice(at + 1), ...items.slice(0, at + 1)];
        const match = ordered.find((item) => itemLabel(item).trim().toLocaleLowerCase().startsWith(key));
        if (!match) return;
        next = items.indexOf(match);
      }
    }
    if (items.length === 0) return;
    event.preventDefault();
    focusItem(items, next);
  }, [close]);

  const onTriggerKeyDown = useCallback((event: ReactKeyboardEvent<HTMLElement>, value: Value) => {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    event.preventDefault();
    show(event.currentTarget, value, event.key === "ArrowUp" ? "last" : "first");
  }, [show]);

  return {
    open: state !== null,
    value: state?.value,
    id,
    show,
    close,
    onTriggerKeyDown,
    menuProps: { ref: menuRef, id, role: "menu", tabIndex: -1, onKeyDown: onMenuKeyDown },
  };
}
