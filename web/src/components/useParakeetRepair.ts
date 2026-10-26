// In-app repair of the managed Parakeet module through the backend setup job.
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { SetupJob } from "../api/types";
import { voiceStatusError } from "./useVoiceRuntimeStatus";

export interface ParakeetRepair {
  job?: SetupJob;
  setupBusy: boolean;
  error: string;
  repair: () => Promise<void>;
  cancel: () => Promise<void>;
}

function setupJobIsActive(job?: SetupJob) {
  return job?.status === "queued" || job?.status === "running";
}

function setupJobIncludesParakeet(job?: SetupJob) {
  if (!job) return false;
  if (job.kind === "parakeet") return true;
  return Boolean(job.steps?.some((step) => step.id === "parakeet" && step.status !== "queued"));
}

export function useParakeetRepair(enabled: boolean, onComplete: () => void): ParakeetRepair {
  const [job, setJob] = useState<SetupJob>();
  const [setupBusy, setSetupBusy] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(false);
  const completedJob = useRef("");
  const issued = useRef(0);
  const applied = useRef(0);
  const onCompleteRef = useRef(onComplete);
  onCompleteRef.current = onComplete;

  // Status reads and repair/cancel calls overlap while a job runs. Apply only
  // the newest answer: a status read sent before Repair must not replace the
  // job Repair returned (which would also end the progress poll).
  const begin = () => ++issued.current;
  const current = (sequence: number) => {
    if (!mounted.current || sequence < applied.current) return false;
    applied.current = sequence;
    return true;
  };

  const refresh = useCallback(async () => {
    const sequence = begin();
    try {
      const response = await api.setupStatus();
      if (!current(sequence)) return;
      const installation = response.installation;
      setSetupBusy(setupJobIsActive(installation));
      const parakeetJob = setupJobIncludesParakeet(installation) ? installation : undefined;
      setJob(parakeetJob);
      setError("");
      if (parakeetJob?.status === "complete" && completedJob.current !== parakeetJob.id) {
        completedJob.current = parakeetJob.id;
        onCompleteRef.current();
      }
    } catch (reason) {
      if (current(sequence)) setError(voiceStatusError(reason));
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void refresh();
  }, [enabled, refresh]);

  useEffect(() => {
    if (!enabled || !setupBusy) return;
    const timer = window.setInterval(() => void refresh(), 1000);
    return () => window.clearInterval(timer);
  }, [enabled, refresh, setupBusy]);

  const repair = useCallback(async () => {
    const sequence = begin();
    setError("");
    try {
      const response = await api.installSetupParakeet();
      if (!current(sequence)) return;
      setJob(response.installation);
      setSetupBusy(true);
    } catch (reason) {
      if (current(sequence)) setError(voiceStatusError(reason));
    }
  }, []);

  const cancel = useCallback(async () => {
    const sequence = begin();
    setError("");
    try {
      const response = await api.cancelSetupInstall();
      if (current(sequence)) setJob(response.installation);
    } catch (reason) {
      if (current(sequence)) setError(voiceStatusError(reason));
    }
  }, []);

  return { job, setupBusy, error, repair, cancel };
}
