// shell-ui.js — app shell navigation: hash router and quick-settings popover.
//
// Routes are flat siblings (docs/ui-design.md): "#/" is the control view and
// "#/settings/<section>" are the settings sections. The quick-settings popover
// is the control bar's explicit quick-settings entry point; it lives in a
// dedicated top layer and is positioned from measured geometry, never from
// viewport units (which can resolve to 0 in embedded/headless contexts).

const SETTINGS_SECTIONS = ["device", "model", "diagnostics"];

const shell = {
  controlView: document.querySelector("#view-control"),
  settingsView: document.querySelector("#view-settings"),
  controlTitle: document.querySelector("#control-title"),
  settingsTitle: document.querySelector("#settings-title"),
  settingsNav: document.querySelector("#settings-nav"),
  sectionLinks: Array.from(document.querySelectorAll("[data-settings-link]")),
  sections: Array.from(document.querySelectorAll("[data-settings-section]")),
  quickButton: document.querySelector("#quick-settings-button"),
  quickPopover: document.querySelector("#quick-popover"),
  quickClose: document.querySelector("#quick-popover-close"),
};

// --- Router --------------------------------------------------------------------

function parseRoute(hash) {
  const parts = String(hash || "").replace(/^#\/?/, "").split("/").filter(Boolean);
  if (parts[0] !== "settings") {
    return { view: "control", section: "" };
  }
  const section = SETTINGS_SECTIONS.includes(parts[1]) ? parts[1] : "device";
  return { view: "settings", section };
}

function applyRoute(options = {}) {
  const route = parseRoute(window.location.hash);
  const settings = route.view === "settings";

  shell.controlView.hidden = settings;
  shell.settingsView.hidden = !settings;
  shell.settingsNav?.setAttribute("data-active", route.section || "");

  for (const section of shell.sections) {
    section.hidden = section.dataset.settingsSection !== route.section;
  }
  for (const link of shell.sectionLinks) {
    if (link.dataset.settingsLink === route.section) {
      link.setAttribute("aria-current", "page");
    } else {
      link.removeAttribute("aria-current");
    }
  }
  const settingsNavLink = document.querySelector("#settings-nav");
  if (settingsNavLink) {
    if (settings) {
      settingsNavLink.setAttribute("aria-current", "page");
    } else {
      settingsNavLink.removeAttribute("aria-current");
    }
  }

  if (options.focusHeading) {
    const heading = settings ? shell.settingsTitle : shell.controlTitle;
    heading?.focus({ preventScroll: true });
  }
}

window.addEventListener("hashchange", () => {
  closeQuickPopover({ restoreFocus: false });
  applyRoute({ focusHeading: true });
});

// --- Quick settings popover ------------------------------------------------------

let quickOpen = false;
let quickPreviousFocus = null;

function positionQuickPopover() {
  const anchor = shell.quickButton.getBoundingClientRect();
  const popover = shell.quickPopover;
  // Explicit pixel width from measured geometry; clientWidth (not innerWidth)
  // so a 0-viewport headless context still yields a usable value.
  const viewportWidth = Math.max(document.documentElement.clientWidth, 320);
  const width = Math.min(380, viewportWidth - 16);
  popover.style.width = `${width}px`;
  const left = Math.max(8, Math.min(anchor.right - width, viewportWidth - width - 8));
  popover.style.left = `${Math.round(left)}px`;
  popover.style.top = `${Math.round(anchor.bottom + 8)}px`;
}

function quickFocusables() {
  return Array.from(
    shell.quickPopover.querySelectorAll("button, input, select, textarea, a[href]"),
  ).filter((element) => !element.disabled && element.offsetParent !== null);
}

function openQuickPopover() {
  if (quickOpen) {
    closeQuickPopover();
    return;
  }
  quickPreviousFocus = document.activeElement;
  shell.quickPopover.hidden = false;
  quickOpen = true;
  shell.quickButton.setAttribute("aria-expanded", "true");
  positionQuickPopover();
  const focusables = quickFocusables();
  (focusables[0] || shell.quickPopover).focus?.({ preventScroll: true });
}

function closeQuickPopover(options = {}) {
  if (!quickOpen) {
    return;
  }
  shell.quickPopover.hidden = true;
  quickOpen = false;
  shell.quickButton.setAttribute("aria-expanded", "false");
  const restore = options.restoreFocus !== false;
  if (restore && quickPreviousFocus && document.contains(quickPreviousFocus)) {
    quickPreviousFocus.focus?.({ preventScroll: true });
  } else if (restore) {
    shell.quickButton.focus?.({ preventScroll: true });
  }
  quickPreviousFocus = null;
}

shell.quickButton?.addEventListener("click", openQuickPopover);
shell.quickClose?.addEventListener("click", () => closeQuickPopover());

// Escape closes the popover when it is open — and only then. The handler runs
// in the capture phase and marks the event consumed so the global
// Escape-stops-motion handler (motion-ui.js) does not also fire. When the
// popover is closed, Escape still reaches the Stop handler unchanged.
document.addEventListener(
  "keydown",
  (event) => {
    if (event.key === "Escape" && quickOpen) {
      event.preventDefault();
      event.stopImmediatePropagation();
      closeQuickPopover();
      return;
    }
    if (event.key === "Tab" && quickOpen) {
      trapQuickFocus(event);
    }
  },
  { capture: true },
);

function trapQuickFocus(event) {
  const focusables = quickFocusables();
  if (!focusables.length) {
    return;
  }
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  const active = document.activeElement;
  if (!shell.quickPopover.contains(active)) {
    event.preventDefault();
    first.focus({ preventScroll: true });
    return;
  }
  if (event.shiftKey && active === first) {
    event.preventDefault();
    last.focus({ preventScroll: true });
  } else if (!event.shiftKey && active === last) {
    event.preventDefault();
    first.focus({ preventScroll: true });
  }
}

document.addEventListener("pointerdown", (event) => {
  if (!quickOpen) {
    return;
  }
  const target = event.target;
  if (shell.quickPopover.contains(target) || shell.quickButton.contains(target)) {
    return;
  }
  closeQuickPopover({ restoreFocus: false });
});

window.addEventListener("resize", () => {
  if (quickOpen) {
    positionQuickPopover();
  }
});
window.addEventListener(
  "scroll",
  () => {
    if (quickOpen) {
      positionQuickPopover();
    }
  },
  { passive: true },
);

applyRoute();
