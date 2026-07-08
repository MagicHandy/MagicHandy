import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import { isOllamaProvider, llmProviderFromSnap } from "../lib/llmStatus";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";

type PlannerApply = {
  enqueued?: number;
  skipped?: number;
  fallback_used?: boolean;
  commands?: unknown[];
  selections?: {
    block_id: string;
    rank_score?: number;
    reasons?: string[];
    bpm?: number;
  }[];
};

export function DiagnosticsPanel() {
  const { t } = useTranslation();
  const { snap } = useStatus();
  const { notify } = useToast();
  const [data, setData] = useState<Record<string, unknown> | null>(null);
  const [ping, setPing] = useState<Record<string, unknown> | null>(null);

  const load = async () => {
    const d = await api.getDiagnostics();
    setData(d);
  };

  useEffect(() => {
    load().catch((e) =>
      notify(e instanceof Error ? e.message : t("common.error"), "error"),
    );
  }, [notify, t]);

  const doPing = async () => {
    try {
      const r = await api.pingOllama();
      setPing(r);
      const provider = r.llm_provider ?? r.provider ?? "llama_cpp";
      const ok = Boolean(r.llm_connected ?? r.ollama_connected ?? r.ok);
      notify(
        ok
          ? isOllamaProvider(provider)
            ? t("diagnostics.ollamaOk")
            : t("diagnostics.llamaCppOk")
          : isOllamaProvider(provider)
            ? t("diagnostics.ollamaFail")
            : t("diagnostics.llamaCppFail"),
        ok ? "ok" : "error",
      );
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const lastApply = data?.last_planner_apply as PlannerApply | undefined;
  const plannerLog = (data?.planner_log_recent ?? []) as Record<string, unknown>[];
  const handyLog = (data?.handy_log_recent ?? []) as Record<string, unknown>[];

  const plannerState = data?.planner_busy
    ? t("diagnostics.plannerBusy", { source: String(data.planner_busy_source ?? "?") })
    : t("diagnostics.plannerFree");

  const llmProvider = snap ? llmProviderFromSnap(snap) : "llama_cpp";
  const pingLabel = isOllamaProvider(llmProvider)
    ? t("diagnostics.pingOllama")
    : t("diagnostics.pingLlamaCpp");
  const pingTitle = isOllamaProvider(llmProvider)
    ? t("diagnostics.pingTitle")
    : t("diagnostics.pingLlamaCppTitle");

  return (
    <div className="diag-layout">
      <div className="btn-row">
        <button type="button" className="btn btn-primary" onClick={doPing}>
          {pingLabel}
        </button>
        <button type="button" className="btn btn-ghost" onClick={load}>
          {t("diagnostics.refresh")}
        </button>
      </div>

      {ping && (
        <section className="glass">
          <h3>{pingTitle}</h3>
          <pre className="json-preview mono">{JSON.stringify(ping, null, 2)}</pre>
        </section>
      )}

      {data && (
        <section className="glass">
          <h3>{t("diagnostics.quickState")}</h3>
          <ul className="hint">
            <li>
              {t("diagnostics.bufferQueue", {
                buffer: String(data.buffer_sec ?? "—"),
                remaining: String(data.buffer_remaining_sec ?? "—"),
              })}
            </li>
            <li>
              {t("diagnostics.playback", {
                state: data.playback_active
                  ? t("diagnostics.playbackActive")
                  : t("diagnostics.playbackStopped"),
                count: String(data.queue_blocks ?? 0),
              })}
            </li>
            <li>
              {t("diagnostics.planner", { state: plannerState })}
              {data.planner_refill_busy ? t("diagnostics.refillBusy") : ""}
            </li>
            <li>
              {t("diagnostics.estop", {
                state: data.emergency_stop ? t("diagnostics.estopYes") : t("diagnostics.estopNo"),
              })}
            </li>
            {data.last_error != null && (
              <li className="handy-log-err">
                {t("diagnostics.error", { msg: String(data.last_error) })}
              </li>
            )}
          </ul>
        </section>
      )}

      {lastApply && (
        <section className="glass">
          <h3>{t("diagnostics.lastApply")}</h3>
          <p className="hint">
            {t("diagnostics.enqueued", {
              n: lastApply.enqueued ?? 0,
              s: lastApply.skipped ?? 0,
            })}
            {lastApply.fallback_used ? t("diagnostics.fallbackUsed") : ""}
          </p>
          {lastApply.commands && lastApply.commands.length > 0 && (
            <pre className="json-preview mono">
              {JSON.stringify(lastApply.commands, null, 2)}
            </pre>
          )}
          {(lastApply.selections?.length ?? 0) > 0 && (
            <div className="selection-list">
              <p className="hint">{t("diagnostics.whyBlocks")}</p>
              {lastApply.selections!.map((sel) => (
                <div key={sel.block_id} className="handy-log-row mono">
                  <span>{sel.block_id.slice(0, 20)}…</span>
                  <span>score {sel.rank_score ?? "—"}</span>
                  <span>{sel.reasons?.join(" · ")}</span>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      {data?.last_planner_json != null && (
        <section className="glass">
          <h3>{t("diagnostics.lastPlannerJson")}</h3>
          <pre className="json-preview mono">
            {JSON.stringify(data.last_planner_json, null, 2)}
          </pre>
        </section>
      )}

      {plannerLog.length > 0 && (
        <section className="glass">
          <h3>{t("diagnostics.plannerAudit")}</h3>
          <p className="hint mono">{String(data?.planner_log_path ?? "")}</p>
          <div className="handy-log-list">
            {plannerLog
              .slice()
              .reverse()
              .slice(0, 25)
              .map((row, i) => (
                <div key={i} className="handy-log-row mono">
                  <span className="handy-log-event">{String(row.event ?? row.type ?? "—")}</span>
                  {row.enqueued != null && <span>+{String(row.enqueued)}</span>}
                  {row.skipped != null && <span>skip {String(row.skipped)}</span>}
                  {row.source != null && <span>{String(row.source)}</span>}
                </div>
              ))}
          </div>
        </section>
      )}

      {handyLog.length > 0 && (
        <section className="glass">
          <h3>{t("diagnostics.handyCommands")}</h3>
          <p className="hint mono">{String(data?.handy_log_path ?? "")}</p>
          <div className="handy-log-list">
            {handyLog
              .slice()
              .reverse()
              .slice(0, 40)
              .map((row, i) => (
                <div key={i} className="handy-log-row mono">
                  <span className="handy-log-event">{String(row.event)}</span>
                  <span>
                    {row.source_filename
                      ? String(row.source_filename)
                      : row.block_id
                        ? String(row.block_id).slice(0, 20)
                        : ""}
                  </span>
                  {row.position_pct != null && (
                    <span>{String(row.position_pct)}%</span>
                  )}
                  {row.duration_ms != null && (
                    <span>{String(row.duration_ms)}ms</span>
                  )}
                  {row.error != null && (
                    <span className="handy-log-err">{String(row.error)}</span>
                  )}
                </div>
              ))}
          </div>
        </section>
      )}

      {!data && <p className="hint center">{t("common.loading")}</p>}
    </div>
  );
}
