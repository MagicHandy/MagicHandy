import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { ModelCheckItem, ModelCheckReport, ModelCheckTurn } from "../api/model-check-types";
import { formatNumber, t, translateKnown } from "../i18n";
import { trapModalTab } from "../util/modal";

interface Props {
  locked: boolean;
  onClose: () => void;
  // Called after the template fix changes so the model list can refresh.
  onModelChanged?: () => void;
}

const message = (error: unknown) => (error instanceof Error ? translateKnown(error.message) : t("Request failed"));
const seconds = (millis: number) => formatNumber(Math.round(millis / 100) / 10);

// The backend decides every verdict; this only words it with the numbers the
// report carries, so the window reads the same in every locale.
export function modelCheckItemText(item: ModelCheckItem, report: ModelCheckReport): { title: string; detail: string } {
  const summary = report.summary;
  const pass = item.status === "pass";
  switch (item.id) {
    case "load":
      return pass
        ? { title: t("Loads and answers"), detail: report.load_millis > 0 ? t("Ready in {seconds} s.", { seconds: seconds(report.load_millis) }) : t("The model was already loaded.") }
        : { title: t("Could not load the model"), detail: report.error ? translateKnown(report.error) : t("Check the model and runtime in the list above.") };
    case "gpu": {
      const layers = { offloaded: report.load_report.offloaded_layers, total: report.load_report.total_layers };
      return pass
        ? { title: t("Runs fully on the GPU"), detail: t("{offloaded} of {total} layers are on the graphics card.", layers) }
        : { title: t("Part of the model runs on the CPU"), detail: t("Only {offloaded} of {total} layers fit on the graphics card, so replies are slower. Choose a smaller model or close other programs that use the GPU.", layers) };
    }
    case "template": {
      const template = report.template;
      if (item.status === "warn") return { title: t("The chat template can make this model think silently"), detail: t("MagicHandy can use a corrected Gemma 4 chat template for this model. Turn the fix on, then run the test again.") };
      if (template?.fix && template.fix_source === "known") return { title: t("Chat template fixed automatically"), detail: t("This is a known model, so MagicHandy applies its Gemma 4 thinking fix when it loads.") };
      if (template?.fix) return { title: t("Chat template fix is on"), detail: t("You turned on the Gemma 4 thinking fix for this model.") };
      return { title: t("Chat template looks right"), detail: t("No known template problem was found.") };
    }
    case "runtime":
      return pass
        ? { title: t("llama.cpp runtime is current"), detail: report.runtime?.version ?? "" }
        : { title: t("A newer llama.cpp runtime is available"), detail: t("Installed {installed}; this release uses {expected}. Update it from General settings, or leave automatic runtime updates on.", { installed: report.runtime?.version || t("none"), expected: report.runtime?.expected ?? "" }) };
    case "reasoning":
      return pass
        ? { title: t("Answers directly"), detail: t("No hidden reasoning before the replies.") }
        : { title: t("Reasons silently before answering"), detail: t("About {chars} characters of hidden reasoning across the test. Replies are slower and can run out of tokens before the reply.", { chars: formatNumber(summary.reasoning_chars) }) };
    case "contract":
      if (pass) return { title: t("Follows the reply format"), detail: t("All {count} replies were valid on the first try.", { count: summary.turns }) };
      if (item.status === "warn") return { title: t("Mostly follows the reply format"), detail: t("Repaired replies: {repaired} of {count}.", { repaired: summary.repaired, count: summary.turns }) };
      return { title: t("Struggles with the reply format"), detail: t("Failed replies: {failed} of {count}; repaired: {repaired}. A tested model from the download list works more reliably.", { failed: summary.failed, count: summary.turns, repaired: summary.repaired }) };
    case "truncation":
      return pass
        ? { title: t("Replies finish within the output limit"), detail: t("No reply was cut off.") }
        : { title: t("Replies were cut off"), detail: t("Replies cut off at the output limit: {count}. Choose a shorter reply length or raise Maximum output.", { count: summary.truncated }) };
    case "speed": {
      const pace = summary.tokens_per_second > 0
        ? t("About {seconds} s per reply, {rate} tokens per second.", { seconds: seconds(summary.average_millis), rate: formatNumber(Math.round(summary.tokens_per_second)) })
        : t("About {seconds} s per reply.", { seconds: seconds(summary.average_millis) });
      if (pass) return { title: t("Replies are quick"), detail: pace };
      if (item.status === "warn") return { title: t("Replies are slow"), detail: pace };
      return { title: t("Replies are very slow"), detail: `${pace} ${t("Choose a smaller model for this graphics card.")}` };
    }
    case "verbosity":
      return pass
        ? { title: t("Reply length fits your setting"), detail: t("About {words} words per reply.", { words: summary.average_words }) }
        : { title: t("Replies run long"), detail: t("About {words} words per reply. Try Reply length: Short.", { words: summary.average_words }) };
    case "refusals":
      return pass
        ? { title: t("Stays in character"), detail: t("No refusals in the adult test turns.") }
        : { title: t("Refuses adult requests"), detail: t("Refused replies: {count}. Choose an uncensored (heretic or abliterated) build.", { count: summary.refused }) };
    default:
      return { title: item.id, detail: "" };
  }
}

function turnFlags(turn: ModelCheckTurn): string {
  const flags = [t("{millis} ms", { millis: formatNumber(turn.millis) })];
  if (turn.words) flags.push(t("{words} words", { words: turn.words }));
  flags.push(turn.failed ? t("failed") : turn.repaired ? t("repaired") : t("valid first try"));
  if (turn.truncated) flags.push(t("cut off"));
  if (turn.reasoning_chars > 0) flags.push(t("{chars} hidden reasoning characters", { chars: formatNumber(turn.reasoning_chars) }));
  if (turn.refused) flags.push(t("refused"));
  if (turn.motion) flags.push(t("motion: {motion}", { motion: turn.motion }));
  return flags.join(" · ");
}

export function ModelCheckDialog({ locked, onClose, onModelChanged }: Props) {
  const [report, setReport] = useState<ModelCheckReport | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState("");
  const dialogRef = useRef<HTMLElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;
  const running = report?.state === "running";

  const refresh = useCallback(async () => {
    try {
      setReport(await api.modelCheck());
    } catch (reason) {
      setError(message(reason));
    }
  }, []);

  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.body.style.overflow = "hidden";
    dialogRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeRef.current();
      else if (dialogRef.current) trapModalTab(event, dialogRef.current);
    };
    document.addEventListener("keydown", onKeyDown);
    void refresh();
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
      returnFocus?.focus();
    };
  }, [refresh]);

  useEffect(() => {
    if (!running) return undefined;
    const timer = window.setInterval(() => void refresh(), 700);
    return () => window.clearInterval(timer);
  }, [refresh, running]);

  const act = async (name: string, action: () => Promise<void>) => {
    setBusy(name);
    setError("");
    try {
      await action();
    } catch (reason) {
      setError(message(reason));
    } finally {
      setBusy("");
    }
  };
  const start = () => act("start", async () => {
    setNotice("");
    setReport(await api.startModelCheck());
  });
  const cancel = () => act("cancel", async () => setReport(await api.cancelModelCheck()));
  const setFix = (enabled: boolean) => act("fix", async () => {
    if (!report?.template?.model_id) return;
    await api.setModelTemplateFix(report.template.model_id, enabled);
    setNotice(enabled ? t("The fix is on. Run the test again to confirm it.") : t("The fix is off. Run the test again to compare."));
    onModelChanged?.();
  });

  const items = (report?.items ?? []).filter((item) => item.status !== "skip");
  const finished = report && (report.state === "complete" || report.state === "failed" || report.state === "canceled");
  const problems = items.filter((item) => item.status !== "pass").length;
  const offer = report?.template?.fix_offer;
  const userFix = report?.template?.fix && report.template.fix_source === "user";
  return (
    <div className="modal-scrim" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section ref={dialogRef} className="model-check-dialog" role="dialog" aria-modal="true" aria-labelledby="model-check-title" aria-describedby="model-check-intro" tabIndex={-1}>
        <header>
          <p className="eyebrow">{report?.model || t("Selected model")}</p>
          <h2 id="model-check-title">{t("Test model")}</h2>
        </header>
        <div className="model-check-body">
          <p id="model-check-intro">{t("Sends a short scripted conversation through the selected model, exactly as chat does, and checks for common problems: silent reasoning, broken replies, cut-off output, slow speed, long replies and refusals. It never moves your device.")}</p>
          {running && (
            <div className="model-check-progress" role="status" aria-live="polite">
              <progress aria-label={t("Model test in progress")} />
              <span>{report.step === "load" ? t("Loading the model...") : t("Test turn {current} of {total}", { current: Math.min(report.turns.length + 1, 5), total: 5 })}</span>
            </div>
          )}
          {report?.state === "canceled" && <p className="form-status">{t("The test was canceled.")}</p>}
          {finished && items.length > 0 && (
            <p className="model-check-verdict" data-tone={problems ? "warn" : "pass"}>
              {problems ? t("Checks needing attention: {count}.", { count: problems }) : t("This model works well with MagicHandy.")}
            </p>
          )}
          {items.length > 0 && report && (
            <ul className="model-check-items">
              {items.map((item) => {
                const text = modelCheckItemText(item, report);
                return (
                  <li key={item.id} data-status={item.status}>
                    <span className="status-dot" data-state={item.status === "pass" ? "ok" : item.status === "warn" ? "warn" : "error"} aria-hidden="true" />
                    <span>
                      <strong>{text.title}<span className="visually-hidden">{t("Result: {status}", { status: item.status === "pass" ? t("passed") : item.status === "warn" ? t("warning") : t("failed") })}</span></strong>
                      {text.detail && <small>{text.detail}</small>}
                      {item.id === "template" && (offer || userFix) && (
                        <button type="button" className="btn btn-secondary" disabled={locked || running || busy !== ""} onClick={() => void setFix(Boolean(offer))}>
                          {offer ? t("Turn on the fix") : t("Turn off the fix")}
                        </button>
                      )}
                    </span>
                  </li>
                );
              })}
            </ul>
          )}
          {notice && <p className="form-status" role="status">{notice}</p>}
          {(report?.turns.length ?? 0) > 0 && (
            <details className="model-check-turns">
              <summary>{t("Replies from the test")}</summary>
              <ol>
                {report?.turns.map((turn, index) => (
                  <li key={index}>
                    <q>{turn.message}</q>
                    <p>{turn.reply || (turn.error ? translateKnown(turn.error) : t("No reply"))}</p>
                    <small>{turnFlags(turn)}</small>
                  </li>
                ))}
              </ol>
            </details>
          )}
          {error && <p className="field-error" role="alert">{error}</p>}
        </div>
        <footer>
          <button type="button" className="btn btn-secondary" onClick={onClose}>{t("Close")}</button>
          {running
            ? <button type="button" className="btn btn-secondary" disabled={locked || busy !== ""} onClick={() => void cancel()}>{t("Stop test")}</button>
            : <button type="button" className="btn btn-primary" disabled={locked || busy !== ""} onClick={() => void start()}>{finished ? t("Run again") : t("Start test")}</button>}
        </footer>
      </section>
    </div>
  );
}
