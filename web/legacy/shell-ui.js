// shell-ui.js — app shell navigation: hash router and the settings window.
//
// The control view (chat + sidebar) is always mounted. Settings opens as one
// window layered over it — chat stays visible behind, the persistent bar and
// Stop stay above and clickable, and nothing ever stacks on the window
// (docs/ui-design.md). Routes stay flat and linkable: "#/" is the control
// view and "#/settings/<section>" opens the window on that section.

const SETTINGS_SECTIONS = ["device", "model", "prompts", "diagnostics"];

const shell = {
  controlTitle: document.querySelector("#control-title"),
  overlay: document.querySelector("#settings-overlay"),
  window: document.querySelector("#settings-window"),
  settingsTitle: document.querySelector("#settings-title"),
  close: document.querySelector("#settings-close"),
  profile: document.querySelector("#profile-button"),
  sectionLinks: Array.from(document.querySelectorAll("[data-settings-link]")),
  sections: Array.from(document.querySelectorAll("[data-settings-section]")),
};

let settingsOpen = false;
let previousFocus = null;

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
  const open = route.view === "settings";

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

  if (open && !settingsOpen) {
    previousFocus = document.activeElement;
    shell.overlay.hidden = false;
    settingsOpen = true;
    shell.profile?.setAttribute("aria-expanded", "true");
    if (options.focus !== false) {
      shell.settingsTitle?.focus({ preventScroll: true });
    }
  } else if (!open && settingsOpen) {
    shell.overlay.hidden = true;
    settingsOpen = false;
    shell.profile?.setAttribute("aria-expanded", "false");
    if (options.focus !== false) {
      if (previousFocus && document.contains(previousFocus) && previousFocus !== document.body) {
        previousFocus.focus?.({ preventScroll: true });
      } else {
        shell.profile?.focus({ preventScroll: true });
      }
    }
    previousFocus = null;
  }
}

window.addEventListener("hashchange", () => applyRoute());

// --- Settings window open/close ---------------------------------------------------

function openSettings() {
  if (settingsOpen) {
    closeSettings();
    return;
  }
  window.location.hash = "#/settings/device";
}

function closeSettings() {
  if (!settingsOpen) {
    return;
  }
  window.location.hash = "#/";
}

shell.profile?.addEventListener("click", openSettings);
shell.close?.addEventListener("click", closeSettings);

// Clicking the dimmed backdrop (not the window itself) closes settings.
shell.overlay?.addEventListener("pointerdown", (event) => {
  if (event.target === shell.overlay) {
    closeSettings();
  }
});

// Escape closes the settings window when it is open — and only then. The
// handler runs in the capture phase and marks the event consumed so the
// global Escape-stops-motion handler (motion-ui.js) does not also fire.
// With the window closed, Escape still reaches the Stop handler unchanged.
document.addEventListener(
  "keydown",
  (event) => {
    if (event.key === "Escape" && settingsOpen) {
      event.preventDefault();
      event.stopImmediatePropagation();
      closeSettings();
      return;
    }
    if (event.key === "Tab" && settingsOpen) {
      trapFocus(event);
    }
  },
  { capture: true },
);

// Keyboard focus cycles within the window while it is open. The bar's Stop
// stays reachable for pointer users always, and for keyboard users via
// Escape (close) then Escape (stop).
function trapFocus(event) {
  const focusables = Array.from(
    shell.window.querySelectorAll("button, input, select, textarea, a[href]"),
  ).filter((element) => !element.disabled && element.offsetParent !== null);
  if (!focusables.length) {
    return;
  }
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  const active = document.activeElement;
  if (!shell.window.contains(active)) {
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

applyRoute({ focus: false });
