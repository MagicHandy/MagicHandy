import { t, translateKnown } from "../i18n";
import { useCallback, useEffect, useState } from "react";
import { ApiError, api } from "../api/client";
import type { RemoteCommandInput, RemoteOutcome, RemoteState } from "../api/remote-types";

export type RemoteSend = (command: RemoteCommandInput) => Promise<string>;

export interface RemoteCommandStatus {
  tone: "waiting" | "error";
  message: string;
}

// A quick desktop answers before this, so the waiting line would only flicker.
const WAITING_NOTICE_MS = 700;

/** Sends commands and follows the newest one until the desktop reports it. */
export function useRemoteCommands(state: RemoteState | null) {
  const [lastID, setLastID] = useState("");
  const [sendError, setSendError] = useState("");
  const [waitingVisible, setWaitingVisible] = useState(false);

  const send = useCallback<RemoteSend>(async (command) => {
    setSendError("");
    try {
      const response = await api.sendRemoteCommand(command);
      setLastID(response.command.id);
      return response.command.id;
    } catch (reason) {
      setLastID("");
      setSendError(remoteSendError(reason));
      return "";
    }
  }, []);

  const outcome = outcomeFor(state, lastID);
  const waiting = Boolean(lastID) && !outcome;
  useEffect(() => {
    setWaitingVisible(false);
    if (!waiting) return;
    const timer = setTimeout(() => setWaitingVisible(true), WAITING_NOTICE_MS);
    return () => clearTimeout(timer);
  }, [waiting, lastID]);

  let status: RemoteCommandStatus | null = null;
  if (sendError) status = { tone: "error", message: sendError };
  else if (outcome && !outcome.ok) status = { tone: "error", message: outcome.error ? translateKnown(outcome.error) : t("The desktop could not do that.") };
  else if (waiting && waitingVisible) status = { tone: "waiting", message: t("Waiting for the desktop…") };
  return { send, status };
}

export function outcomeFor(state: RemoteState | null, id: string): RemoteOutcome | undefined {
  return id ? state?.recent.find((entry) => entry.command_id === id) : undefined;
}

export function remoteSendError(reason: unknown): string {
  const body = reason instanceof ApiError ? reason.body : undefined;
  const code = body && typeof body === "object" && "code" in body ? String(body.code) : "";
  switch (code) {
    case "no_desktop": return t("No desktop is taking commands.");
    case "target_unavailable": return t("The desktop is not showing that right now.");
    case "other_account": return t("The desktop is signed in to another account.");
    case "busy": return t("Too many commands are waiting. Try again in a moment.");
    case "invalid": return t("The desktop does not know that command.");
    default: return reason instanceof Error ? translateKnown(reason.message) : t("The command could not be sent.");
  }
}
