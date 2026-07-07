const STORAGE_KEY = "magichandy.controller.client_id";

function stableClientID(key: string, prefix: string): string {
  try {
    const existing = window.localStorage.getItem(key);
    if (existing) return existing;
    const generated = `${prefix}-${crypto.randomUUID()}`;
    window.localStorage.setItem(key, generated);
    return generated;
  } catch {
    return `${prefix}-${Date.now()}-${Math.round(Math.random() * 100000)}`;
  }
}

export function controllerClientID(): string {
  return stableClientID(STORAGE_KEY, "browser");
}

export function controllerHeaders(): Record<string, string> {
  return {
    "X-MagicHandy-Client-ID": controllerClientID(),
  };
}
