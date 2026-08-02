import { useEffect, useRef } from "react";
import { api } from "../api/client";
import type { MediaJobState, MediaScanState } from "../api/types";
import { formatNumber, t } from "../i18n";
import { useAppState, useNotifications, type NotificationDraft } from "../state/app-state";
import { BellIcon, CheckIcon, CloseIcon, TrashIcon } from "./icons";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  restoreFocusOnClose?: boolean;
}

export function NotificationCenter({ open, onOpenChange, restoreFocusOnClose = true }: Props) {
  const { backendOnline, startupError, state } = useAppState();
  const { items, unreadCount, push, markRead, markAllRead, clear } = useNotifications();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);
  const previousBackend = useRef(backendOnline);

  const scan = state?.media?.scan;
  const job = state?.media?.job;
  const voiceSettings = state?.settings?.voice;
  const voiceWorkers = state?.voice?.workers;
  const voiceCrashed = Boolean(
    voiceSettings?.enabled &&
    (voiceWorkers?.tts?.state === "crashed" || voiceWorkers?.asr?.state === "crashed"),
  );
  const speakNotReady = Boolean(
    voiceSettings?.enabled &&
    voiceSettings.speak_replies &&
    voiceSettings.tts_provider &&
    voiceSettings.tts_provider !== "none" &&
    !(voiceWorkers?.tts?.state === "running" && voiceWorkers?.tts?.model_state === "ready"),
  );

  useEffect(() => {
    if (open) closeRef.current?.focus();
    else if (wasOpen.current && restoreFocusOnClose) triggerRef.current?.focus();
    wasOpen.current = open;
  }, [open, restoreFocusOnClose]);

  useEffect(() => {
    if (previousBackend.current && !backendOnline) {
      push({
        title: t("Core connection lost"),
        detail: startupError || t("Backend-required controls are locked until the core responds."),
        category: "system",
        tone: "error",
      });
    } else if (!previousBackend.current && backendOnline) {
      push({
        title: t("Core connection restored"),
        category: "system",
        tone: "success",
      });
    }
    previousBackend.current = backendOnline;
  }, [backendOnline, push, startupError]);

  useEffect(() => {
    const draft = scanNotification(scan);
    if (draft) push(draft);
  }, [push, scan]);

  useEffect(() => {
    const draft = jobNotification(job);
    if (draft) push(draft);
  }, [job, push]);

  useEffect(() => {
    for (const role of ["tts", "asr"] as const) {
      const worker = voiceWorkers?.[role];
      if (worker?.state !== "crashed") continue;
      push({
        title: t("Voice worker crashed"),
        detail: `${role === "tts" ? t("Speech output") : t("Speech input")}. ${t("Open Voice settings for diagnostics and recovery.")}`,
        category: "voice",
        tone: "error",
        href: "#/settings/voice",
        sourceKey: `voice-worker-crashed:${role}:${worker.started_at || "current"}`,
      });
    }
  }, [push, voiceWorkers]);

  const setupComplete = state?.settings?.ui?.setup_completed !== false;
  const automaticUpdateChecks = state?.settings?.ui?.update_check_mode !== "manual";
  const releaseBuild = /^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(state?.version?.trim() ?? "");
  useEffect(() => {
    if (!backendOnline || !setupComplete || !automaticUpdateChecks || !releaseBuild) return;
    let cancelled = false;
    void api.updateStatus().then((status) => {
      if (cancelled || status.state !== "available" || !status.latest) return;
      push({
        title: t("New MagicHandy release"),
        detail: t("{version} is available. Open General settings to review the release.", { version: status.latest.version || status.latest.tag }),
        category: "updates",
        tone: "info",
        href: "#/settings/general",
        sourceKey: `update-available:${status.latest.tag}`,
      });
    }).catch(() => {
      // Startup update checks are advisory; manual checks surface failures.
    });
    return () => {
      cancelled = true;
    };
  }, [automaticUpdateChecks, backendOnline, push, releaseBuild, setupComplete]);

  const active = scan?.running === true || job?.running === true;
  const attention = !backendOnline || voiceCrashed || speakNotReady;
  const badge = unreadCount > 99 ? "99+" : String(unreadCount);
  const label = unreadCount > 0
    ? t("Notifications, {count} unread", { count: formatNumber(unreadCount) })
    : active ? t("Notifications, activity in progress") : t("Notifications");

  return (
    <div className="notification-center" data-open={open} data-active={active}>
      {open && <section
        id="notification-center-panel"
        className="notification-center-panel"
        aria-label={t("Notifications")}
      >
        <header className="notification-center-head">
          <div>
            <h2>{t("Notifications")}</h2>
            <p>{unreadCount > 0 ? t("{count} unread", { count: formatNumber(unreadCount) }) : t("Up to date")}</p>
          </div>
          <div className="notification-head-actions">
            <div className="notification-management-actions">
              <button type="button" className="icon-button" disabled={unreadCount === 0} aria-label={t("Mark all notifications read")} title={t("Mark all read")} onClick={markAllRead}>
                <CheckIcon />
              </button>
              <button type="button" className="icon-button" disabled={items.length === 0} aria-label={t("Clear notification history")} title={t("Clear history")} onClick={clear}>
                <TrashIcon />
              </button>
            </div>
            <button ref={closeRef} type="button" className="icon-button" aria-label={t("Close notifications")} onClick={() => onOpenChange(false)}>
              <CloseIcon />
            </button>
          </div>
        </header>

        {active && (
          <div className="notification-section">
            <h3>{t("Activity")}</h3>
            {scan?.running && <ScanActivity scan={scan} onNavigate={() => onOpenChange(false)} />}
            {job?.running && <JobActivity job={job} onNavigate={() => onOpenChange(false)} />}
          </div>
        )}

        {attention && (
          <div className="notification-section">
            <h3>{t("Attention")}</h3>
            {!backendOnline && (
              <div className="notification-live-row" data-tone="error">
                <span className="status-dot" data-state="warn" />
                <span><strong>{t("Core offline")}</strong><small>{startupError || t("Backend-required controls are locked.")}</small></span>
              </div>
            )}
            {voiceCrashed && (
              <a className="notification-live-row" data-tone="error" href="#/settings/voice" onClick={() => onOpenChange(false)}>
                <span className="status-dot" data-state="warn" />
                <span><strong>{t("Voice worker crashed")}</strong><small>{t("Open Voice settings for diagnostics and recovery.")}</small></span>
              </a>
            )}
            {!voiceCrashed && speakNotReady && (
              <a className="notification-live-row" data-tone="warning" href="#/settings/voice" onClick={() => onOpenChange(false)}>
                <span className="status-dot" data-state="warn" />
                <span><strong>{t("Voice output is not ready")}</strong><small>{t("Speak replies is on, but the TTS model is unavailable.")}</small></span>
              </a>
            )}
          </div>
        )}

        <div className="notification-section notification-history">
          <h3>{t("Recent")}</h3>
          {items.length === 0
            ? <p className="notification-empty">{t("No recent notifications.")}</p>
            : items.map((item) => {
              const content = (
                <>
                  <span className="notification-item-marker" data-tone={item.tone} aria-hidden="true" />
                  <span className="notification-item-copy">
                    <strong>{item.title}</strong>
                    {item.detail && <small>{item.detail}</small>}
                    <time dateTime={item.createdAt}>{formatNotificationTime(item.createdAt)}</time>
                  </span>
                  {!item.read && <span className="notification-unread-dot" aria-label={t("Unread")} />}
                </>
              );
              return item.href
                ? <a key={item.id} className="notification-item" data-read={item.read} href={item.href} onClick={() => { markRead(item.id); onOpenChange(false); }}>{content}</a>
                : <button key={item.id} type="button" className="notification-item" data-read={item.read} onClick={() => markRead(item.id)}>{content}</button>;
            })}
        </div>
      </section>}

      <button
        ref={triggerRef}
        type="button"
        className="notification-trigger icon-button"
        aria-label={label}
        aria-controls={open ? "notification-center-panel" : undefined}
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
      >
        <BellIcon />
        {unreadCount > 0 && <span className="notification-badge" aria-hidden="true">{badge}</span>}
        {unreadCount === 0 && active && <span className="notification-activity-dot" aria-hidden="true" />}
      </button>
    </div>
  );
}

function ScanActivity({ scan, onNavigate }: { scan: MediaScanState; onNavigate: () => void }) {
  return (
    <a className="notification-activity-row" href="#/settings/media" onClick={onNavigate}>
      <span className="status-dot" data-state="active" />
      <span>
        <strong>{scan.trigger === "startup" ? t("Startup library scan") : t("Library scan")}</strong>
        <small>{t("{files} files checked · {videos} videos found", { files: formatNumber(scan.files_visited), videos: formatNumber(scan.videos_found) })}</small>
      </span>
      <progress aria-label={t("Library scan in progress")} />
    </a>
  );
}

function JobActivity({ job, onNavigate }: { job: MediaJobState; onNavigate: () => void }) {
  const isConversion = job.kind === "conversion";
  const progress = job.total > 0 ? Math.min(job.total, job.processed) : 0;
  return (
    <a className="notification-activity-row" href="#/settings/media" onClick={onNavigate}>
      <span className="status-dot" data-state="active" />
      <span>
        <strong>{isConversion ? t("Converting media") : t("Generating thumbnails")}</strong>
        <small>{isConversion && job.current_name
          ? t("{name} · {done} of {total}", { name: job.current_name, done: formatNumber(job.processed + 1), total: formatNumber(job.total) })
          : t("{done} of {total}", { done: formatNumber(job.processed), total: formatNumber(job.total) })}</small>
      </span>
      <progress aria-label={t("Media task progress")} max={Math.max(job.total, 1)} value={progress} />
    </a>
  );
}

function scanNotification(scan?: MediaScanState): NotificationDraft | null {
  if (!scan?.started_at || scan.running) return null;
  if (!scan.completed_at) return null;
  const issues = scan.summary?.issues?.length ?? 0;
  const tone = scan.error ? "error" : issues > 0 ? "warning" : scan.cancelled ? "info" : "success";
  const title = scan.error
    ? t("Library scan failed")
    : scan.cancelled ? t("Library scan cancelled")
      : issues > 0 ? t("Library scan completed with warnings") : t("Library scan complete");
  const summary = scan.summary;
  const detail = summary
    ? t("{added} added · {updated} updated · {removed} removed · {missing} marked missing", {
      added: formatNumber(summary.added),
      updated: formatNumber(summary.updated),
      removed: formatNumber(summary.removed),
      missing: formatNumber(summary.missing),
    })
    : scan.error;
  return {
    title,
    detail: scan.error || detail,
    category: "library",
    tone,
    href: "#/settings/media",
    sourceKey: `scan-complete:${scan.completed_at}`,
  };
}

function jobNotification(job?: MediaJobState): NotificationDraft | null {
  if (!job?.started_at || job.running) return null;
  const name = job.kind === "conversion" ? t("Media conversion") : t("Thumbnail generation");
  if (!job.completed_at) return null;
  const failed = job.failed > 0 || Boolean(job.error);
  return {
    title: job.cancelled
      ? t("{name} cancelled", { name })
      : failed ? t("{name} completed with errors", { name }) : t("{name} complete", { name }),
    detail: job.error || t("{done} completed · {failed} failed", { done: formatNumber(job.succeeded), failed: formatNumber(job.failed) }),
    category: "library",
    tone: failed ? "error" : job.cancelled ? "info" : "success",
    href: "#/settings/media",
    sourceKey: `media-job-complete:${job.completed_at}`,
  };
}

function formatNotificationTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(date);
}
