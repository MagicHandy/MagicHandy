import { t } from "../i18n";

// Confirmations render inside the app (ConfirmHost) instead of the browser's
// window.confirm, which blocks the page — including Emergency Stop and its
// Escape shortcut — until it is answered. Callback style keeps the outcome
// synchronous when no host is mounted, as in isolated component tests, where
// the browser dialog remains the fallback.

export interface ConfirmOptions {
  confirmLabel?: string;
  destructive?: boolean;
}

export interface ConfirmRequest {
  id: number;
  message: string;
  confirmLabel: string;
  destructive: boolean;
  resolve: (confirmed: boolean) => void;
}

let host: ((request: ConfirmRequest) => void) | null = null;
let nextID = 1;

export function registerConfirmHost(show: (request: ConfirmRequest) => void): () => void {
  host = show;
  return () => {
    if (host === show) host = null;
  };
}

/** Runs proceed only after the person confirms message. */
export function confirmThen(message: string, options: ConfirmOptions, proceed: () => void): void {
  const show = host;
  if (!show) {
    if (window.confirm(message)) proceed();
    return;
  }
  show({
    id: nextID++,
    message,
    confirmLabel: options.confirmLabel ?? t("Continue"),
    destructive: Boolean(options.destructive),
    resolve: (confirmed) => {
      if (confirmed) proceed();
    },
  });
}
