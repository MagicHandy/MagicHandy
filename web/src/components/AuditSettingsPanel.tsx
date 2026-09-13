import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { AccessAuditEvent, AccessAuditPage } from "../api/audit-types";
import type { UserAccount } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";

const eventLabels: Record<string, MessageKey> = {
  account_created: "Account created", account_enabled: "Account enabled", account_disabled: "Account disabled",
  password_changed: "Password changed", session_created: "Signed in", session_revoked: "Session signed out",
  sessions_revoked: "Other sessions signed out", grant_issued: "Control permission granted", grant_revoked: "Control permission revoked",
  login_failed: "Sign-in rejected", login_throttled: "Sign-in throttled", control_claimed: "Control claimed",
  control_transferred: "Control transferred", control_lost: "Control lost", command_finished: "Command completed",
  stop_finished: "Stop result", server_started: "Core started", server_stopped: "Core shut down", history_gap: "History incomplete",
};
const resultLabels: Record<string, MessageKey> = {
  success: "Succeeded", rejected: "Rejected", failed: "Failed", unconfirmed: "Unconfirmed", unknown: "Unknown", incomplete: "Incomplete",
};
const operationLabels: Record<string, MessageKey> = {
  motion: "Motion", mode: "Mode", chat: "Chat", media: "Media", feedback: "Feedback", preferences: "Preferences", host: "Host operation",
  heartbeat_expired: "Heartbeat expired", session_ended: "Session ended", permission_ended: "Control permission ended", takeover: "Takeover", emergency: "Emergency Stop", shutdown: "Shutdown", other: "Other",
};

export function AuditSettingsPanel({ backendOnline, accounts }: { backendOnline: boolean; accounts: UserAccount[] }) {
  const [expanded, setExpanded] = useState(false);
  const [before, setBefore] = useState(0);
  const [revision, setRevision] = useState(0);
  const [page, setPage] = useState<AccessAuditPage | null>(null);
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState("");
  const downloadRequest = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!expanded || !backendOnline) return;
    const request = new AbortController();
    let active = true;
    const timeout = window.setTimeout(() => {
      request.abort();
      if (active) { setError(t("Access history is unavailable.")); setLoading(false); }
    }, 10_000);
    setLoading(true);
    setError("");
    void api.accessAudit(before, request.signal).then((next) => {
      if (!request.signal.aborted) setPage(next);
    }).catch((reason: unknown) => {
      if (active) setError(request.signal.aborted ? t("Access history is unavailable.") : reason instanceof Error ? translateKnown(reason.message) : t("Access history is unavailable."));
    }).finally(() => {
      window.clearTimeout(timeout);
      if (active) setLoading(false);
    });
    return () => { active = false; window.clearTimeout(timeout); request.abort(); };
  }, [expanded, backendOnline, before, revision]);

  useEffect(() => {
    setDownloading(false);
    return () => { downloadRequest.current?.abort(); downloadRequest.current = null; };
  }, [backendOnline, expanded]);

  async function downloadPage() {
    if (!page || !backendOnline || loading || downloading || !expanded) return;
    const request = new AbortController();
    downloadRequest.current = request;
    const timeout = window.setTimeout(() => {
      request.abort();
      if (downloadRequest.current === request) { setError(t("Access history is unavailable.")); setDownloading(false); }
    }, 10_000);
    setDownloading(true);
    setError("");
    try {
      const anchor = page.events[0] ? page.events[0].sequence + 1 : 0;
      const report = await api.exportAccessAudit(anchor, request.signal);
      if (request.signal.aborted) return;
      const url = URL.createObjectURL(new Blob([JSON.stringify(report, null, 2) + "\n"], { type: "application/json" }));
      const link = document.createElement("a");
      link.href = url;
      link.download = "magichandy-access-history.json";
      link.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (reason) {
      if (downloadRequest.current === request) setError(request.signal.aborted ? t("Access history is unavailable.") : reason instanceof Error ? translateKnown(reason.message) : t("Access history is unavailable."));
    } finally {
      window.clearTimeout(timeout);
      if (downloadRequest.current === request) { downloadRequest.current = null; setDownloading(false); }
    }
  }

  const locked = !backendOnline || loading || downloading;
  const accountName = (id?: string) => accounts.find((account) => account.id === id)?.username || (id ? t("Account {id}", { id: id.slice(0, 8) }) : "—");
  return <details className="group access-audit" onToggle={(event) => setExpanded(event.currentTarget.open)}>
    <summary className="group-title">{t("Access and control history")}</summary>
    {expanded && <>
      <p className="hint-block">{t("Administrator history records access and control outcomes without passwords, chat, audio, or private file paths.")}</p>
      <div className="audit-toolbar">
        <button className="btn btn-secondary" disabled={!backendOnline || downloading} onClick={() => { setBefore(0); setRevision((value) => value + 1); }}>{t("Refresh history")}</button>
        <button className="btn btn-secondary" disabled={locked || !page?.events.length} onClick={() => void downloadPage()}>{t("Download this page")}</button>
        {before > 0 && <button className="btn btn-secondary" disabled={locked} onClick={() => setBefore(0)}>{t("Newest events")}</button>}
        <button className="btn btn-secondary" disabled={locked || !page?.has_more} onClick={() => page && setBefore(page.next_before)}>{t("Older events")}</button>
      </div>
      {!backendOnline && <p className="form-status" role="status">{t("Reconnect to read access history.")}</p>}
      {loading && <p className="form-status" role="status">{t("Loading access history…")}</p>}
      {error && <p className="form-status" role="alert">{error}</p>}
      {page && <>
        <p className="hint-block">{t("Up to {rows} events are retained for {days} days. This page and its download contain at most {limit} events.", { rows: page.row_limit, days: page.retention_days, limit: page.limit })}</p>
        {(page.writer.dropped_since_startup > 0 || !page.writer.storage_available) && <p className="form-status" role="alert">{t("Audit history may be incomplete: {count} events were dropped since startup. Device Stop remains independent of audit storage.", { count: page.writer.dropped_since_startup })}</p>}
        {page.events.length === 0 ? <p className="form-status">{t("No retained access events on this page.")}</p> : <ol className="audit-events" aria-label={t("Access events")}>
          {page.events.map((event) => <li key={event.id}>
            <div className="audit-event-heading"><strong>{translateKnown(eventLabels[event.kind] || "Unknown")}</strong><span>{translateKnown(resultLabels[event.outcome] || "Unknown")}</span></div>
            <div className="audit-event-meta"><time dateTime={new Date(event.occurred_at_ms).toISOString()}>{new Date(event.occurred_at_ms).toLocaleString()}</time><span>{event.actor.type === "account" ? accountName(event.actor.account_id) : event.actor.type === "public" ? t("Unauthenticated caller") : event.actor.type === "local" ? t("Local client") : t("System")}</span></div>
            {event.operation && <span>{translateKnown(operationLabels[event.operation] || "Unknown")}</span>}
            {event.target_account_id && <span> · {accountName(event.target_account_id)}</span>}
            {event.count !== undefined && <span> · {event.kind === "history_gap" ? t("{count} missing events", { count: event.count }) : t("{count} sessions", { count: event.count })}</span>}
            {event.expires_at_ms !== undefined && <span> · {t("Expires: {time}", { time: new Date(event.expires_at_ms).toLocaleString() })}</span>}
            <AuditReferences event={event} />
          </li>)}
        </ol>}
      </>}
    </>}
  </details>;
}

function AuditReferences({ event }: { event: AccessAuditEvent }) {
  return <details className="audit-references"><summary>{t("Correlation details")}</summary><dl>
    <dt>{t("Event")}</dt><dd>{event.sequence} · {event.id}</dd>
    {event.actor.session_id && <><dt>{t("Signed-in session")}</dt><dd>{event.actor.session_id}</dd></>}
    {event.target_session_id && <><dt>{t("Affected session")}</dt><dd>{event.target_session_id}</dd></>}
    {event.grant_id && <><dt>{t("Control permission")}</dt><dd>{event.grant_id}</dd></>}
    {event.epoch && <><dt>{t("Core run")}</dt><dd>{event.epoch}</dd></>}
    {event.generation !== undefined && <><dt>{t("Control generation")}</dt><dd>{event.generation}</dd></>}
    {event.stop_sequence !== undefined && <><dt>{t("Stop sequence")}</dt><dd>{event.stop_sequence}</dd></>}
    {event.trace_sequence !== undefined && <><dt>{t("Trace sequence")}</dt><dd>{event.trace_sequence}</dd></>}
    {event.correlation && <><dt>{t("Command reference")}</dt><dd>{event.correlation}</dd></>}
    {event.http_status !== undefined && <><dt>{t("HTTP status")}</dt><dd>{event.http_status}</dd></>}
  </dl></details>;
}
