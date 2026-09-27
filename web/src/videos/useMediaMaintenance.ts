import { t } from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaJobState, MediaScanState, MediaToolStatus } from "../api/types";

interface Options {
  hostAdministration: boolean;
  /** Controller or host permission is missing; maintenance stays read-only. */
  hostLocked: boolean;
  /** Called when a scan or job finishes and the catalog may have changed. */
  onCatalogChanged: () => void;
}

export interface MediaMaintenance {
  scan: MediaScanState | null;
  scanError: string;
  scanAction: "start" | "cancel" | "";
  startScan: () => Promise<void>;
  cancelScan: () => Promise<void>;
  job: MediaJobState | null;
  tools: MediaToolStatus | null;
  conversionError: string;
  setConversionError: (message: string) => void;
  /** Starts a repair job and returns it, or null when it could not start. */
  requestConversion: (ids: string[]) => Promise<MediaJobState | null>;
  cancelJob: () => Promise<void>;
}

// Explicit library work: filesystem scans, conversion and thumbnail jobs.
// Status polling runs only while work is running, and a transient status
// failure never hides the catalog that is already loaded.
export function useMediaMaintenance({ hostAdministration, hostLocked, onCatalogChanged }: Options): MediaMaintenance {
  const [scan, setScan] = useState<MediaScanState | null>(null);
  const [scanError, setScanError] = useState("");
  const [scanAction, setScanAction] = useState<"start" | "cancel" | "">("");
  const [job, setJob] = useState<MediaJobState | null>(null);
  const [tools, setTools] = useState<MediaToolStatus | null>(null);
  const [conversionError, setConversionError] = useState("");
  const mounted = useRef(true);
  const catalogChanged = useRef(onCatalogChanged);
  catalogChanged.current = onCatalogChanged;

  const updateJob = useCallback((next: MediaJobState) => {
    setJob((current) => {
      // A status request issued before a new job started may resolve late.
      // Never let that older snapshot stop polling the job just accepted.
      if (current?.running && current.started_at && next.started_at !== current.started_at) return current;
      return next;
    });
  }, []);

  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void api.mediaScan(controller.signal).then((response) => {
      if (mounted.current && !controller.signal.aborted) {
        setScan(response.scan);
        setScanError("");
      }
    }).catch((reason) => {
      if (mounted.current && !controller.signal.aborted) {
        setScanError(reason instanceof Error ? reason.message : t("Scan status could not be loaded."));
      }
    });
    if (!hostAdministration) setTools(null);
    if (hostAdministration) void api.mediaTools(controller.signal).then((response) => {
      if (mounted.current && !controller.signal.aborted) setTools(response.tools);
    }).catch(() => {
      // The absent state is the honest default: without a tools answer,
      // conversion stays offered but disabled with a reason.
    });
    void api.mediaJob(controller.signal).then((response) => {
      if (mounted.current && !controller.signal.aborted) updateJob(response.job);
    }).catch(() => {});
    return () => {
      mounted.current = false;
      controller.abort();
    };
  }, [hostAdministration, updateJob]);

  useEffect(() => {
    if (!scan?.running) return undefined;
    let stopped = false;
    let timer: number | undefined;
    const schedule = (delay: number) => {
      timer = window.setTimeout(() => void poll(), delay);
    };
    const poll = async () => {
      try {
        const response = await api.mediaScan();
        if (stopped || !mounted.current) return;
        setScan(response.scan);
        setScanError("");
        if (response.scan.running) schedule(500);
        else catalogChanged.current();
      } catch (reason) {
        if (stopped || !mounted.current) return;
        setScanError(reason instanceof Error ? reason.message : t("Scan status could not be loaded."));
        schedule(1500);
      }
    };
    schedule(500);
    return () => {
      stopped = true;
      window.clearTimeout(timer);
    };
  }, [scan?.running]);

  useEffect(() => {
    if (!job?.running) return undefined;
    let stopped = false;
    let timer: number | undefined;
    const poll = async () => {
      try {
        const response = await api.mediaJob();
        if (stopped || !mounted.current) return;
        updateJob(response.job);
        if (response.job.running) timer = window.setTimeout(() => void poll(), 700);
        else catalogChanged.current();
      } catch {
        if (!stopped && mounted.current) timer = window.setTimeout(() => void poll(), 2000);
      }
    };
    timer = window.setTimeout(() => void poll(), 700);
    return () => {
      stopped = true;
      window.clearTimeout(timer);
    };
  }, [job?.running, updateJob]);

  async function startScan() {
    if (hostLocked) return;
    setScanError("");
    setScanAction("start");
    try {
      const response = await api.startMediaScan();
      if (mounted.current) setScan(response.scan);
    } catch (reason) {
      if (mounted.current) setScanError(reason instanceof Error ? reason.message : t("Video scan could not be started."));
    } finally {
      if (mounted.current) setScanAction("");
    }
  }

  async function cancelScan() {
    if (hostLocked) return;
    setScanError("");
    setScanAction("cancel");
    try {
      const response = await api.cancelMediaScan();
      if (mounted.current) setScan(response.scan);
    } catch (reason) {
      if (mounted.current) setScanError(reason instanceof Error ? reason.message : t("Video scan could not be cancelled."));
    } finally {
      if (mounted.current) setScanAction("");
    }
  }

  // Repairs files that cannot play: named identifiers or the whole library.
  // Either way the server converts only what it has established is broken,
  // so this cannot re-encode a working file.
  async function requestConversion(ids: string[]): Promise<MediaJobState | null> {
    if (hostLocked) return null;
    setConversionError("");
    try {
      const response = await api.convertMedia(ids);
      if (!mounted.current) return null;
      updateJob(response.job);
      if (!response.job.running) catalogChanged.current();
      return response.job;
    } catch (reason) {
      if (mounted.current) setConversionError(reason instanceof Error ? reason.message : t("Conversion could not be started."));
      return null;
    }
  }

  async function cancelJob() {
    if (hostLocked) return;
    try {
      const response = await api.cancelMediaJob();
      if (mounted.current) updateJob(response.job);
    } catch {
      // Cancellation is advisory; the next poll reports the real state.
    }
  }

  return { scan, scanError, scanAction, startScan, cancelScan, job, tools, conversionError, setConversionError, requestConversion, cancelJob };
}
