import { useTranslation } from "react-i18next";
import { useCallback, useEffect, useState } from "react";
import { api, downloadText } from "../api/client";
import type { MotionBlock } from "../api/types";
import { useToast } from "../hooks/useToast";

const PAGE_SIZE = 20;

export function Patterns() {
  const { t } = useTranslation();
  const { toast, notify } = useToast();
  const [blocks, setBlocks] = useState<MotionBlock[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [zone, setZone] = useState("");
  const [speed, setSpeed] = useState("");
  const [minInt, setMinInt] = useState(0);
  const [maxInt, setMaxInt] = useState(100);
  const [favOnly, setFavOnly] = useState(false);
  const [confirmTest, setConfirmTest] = useState<string | null>(null);

  const filters = useCallback(
    () => ({
      zone,
      speed,
      min_intensity: minInt,
      max_intensity: maxInt,
      favorites_only: favOnly,
      offset: (page - 1) * PAGE_SIZE,
      limit: PAGE_SIZE,
    }),
    [zone, speed, minInt, maxInt, favOnly, page],
  );

  const load = useCallback(async () => {
    const f = filters();
    const [listRes, countRes] = await Promise.all([
      api.listPatterns(f),
      api.countPatterns(f),
    ]);
    setBlocks(listRes.blocks);
    setTotal(countRes.total);
  }, [filters]);

  useEffect(() => {
    load().catch((e) =>
      notify(e instanceof Error ? e.message : t("common.error"), "error"),
    );
  }, [load, notify, t]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const feedback = async (id: string, kind: string) => {
    try {
      const r = await api.patternFeedback(id, kind);
      notify(
        t("patterns.feedbackScore", {
          kind,
          score: r.success_score?.toFixed(2) ?? "—",
        }),
        "ok",
      );
      await load();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const doExport = async (id: string, fmt: string) => {
    try {
      const { filename, content } = await api.exportPattern(id, fmt);
      downloadText(filename, content);
      notify(t("patterns.download", { filename }), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const runMockTest = async (id: string) => {
    try {
      const r = await api.testPatternMock(id);
      notify(t("patterns.mockOk", { count: r.actions_played ?? 0 }), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setConfirmTest(null);
    }
  };

  return (
    <div>
      <h2>{t("patterns.title")}</h2>

      <div className="panel row">
        <div className="field" style={{ marginBottom: 0 }}>
          <label>{t("patterns.colZone")}</label>
          <input value={zone} onChange={(e) => setZone(e.target.value)} />
        </div>
        <div className="field" style={{ marginBottom: 0 }}>
          <label>{t("patterns.colSpeed")}</label>
          <input value={speed} onChange={(e) => setSpeed(e.target.value)} />
        </div>
        <div className="field" style={{ marginBottom: 0 }}>
          <label>{t("patterns.minInt")}</label>
          <input
            type="number"
            value={minInt}
            onChange={(e) => setMinInt(Number(e.target.value))}
          />
        </div>
        <div className="field" style={{ marginBottom: 0 }}>
          <label>{t("patterns.maxInt")}</label>
          <input
            type="number"
            value={maxInt}
            onChange={(e) => setMaxInt(Number(e.target.value))}
          />
        </div>
        <label className="row" style={{ gap: 4 }}>
          <input
            type="checkbox"
            checked={favOnly}
            onChange={(e) => setFavOnly(e.target.checked)}
          />
          {t("patterns.favoritesOnly")}
        </label>
        <button
          type="button"
          className="btn secondary"
          onClick={() => {
            setPage(1);
            load().catch((e) =>
              notify(e instanceof Error ? e.message : t("common.error"), "error"),
            );
          }}
        >
          {t("patterns.filter")}
        </button>
      </div>

      <p className="muted">
        {t("patterns.pageInfo", { total, page, totalPages })}
      </p>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{t("patterns.colId")}</th>
              <th>{t("patterns.colZone")}</th>
              <th>{t("patterns.colSpeed")}</th>
              <th>{t("patterns.colDuration")}</th>
              <th>{t("patterns.colScore")}</th>
              <th>{t("patterns.colActions")}</th>
            </tr>
          </thead>
          <tbody>
            {blocks.map((b) => (
              <tr key={b.id}>
                <td title={b.id}>{b.id.slice(0, 12)}…</td>
                <td>{b.zone ?? "—"}</td>
                <td>{b.speed ?? "—"}</td>
                <td>{b.duration_ms}ms</td>
                <td>{(b.success_score ?? 0).toFixed(2)}</td>
                <td>
                  <div className="row" style={{ gap: 4 }}>
                    {["like", "dislike", "favorite", "block"].map((k) => (
                      <button
                        key={k}
                        type="button"
                        className="btn secondary"
                        style={{ padding: "2px 6px", fontSize: "0.7rem" }}
                        onClick={() => feedback(b.id, k)}
                      >
                        {k}
                      </button>
                    ))}
                    <button
                      type="button"
                      className="btn secondary"
                      style={{ padding: "2px 6px", fontSize: "0.7rem" }}
                      onClick={() => setConfirmTest(b.id)}
                    >
                      mock
                    </button>
                    <select
                      defaultValue=""
                      onChange={(e) => {
                        if (e.target.value) doExport(b.id, e.target.value);
                        e.target.value = "";
                      }}
                      style={{ fontSize: "0.7rem" }}
                    >
                      <option value="">{t("common.export")}…</option>
                      <option value="funscript">funscript</option>
                      <option value="csv">csv</option>
                      <option value="json">json</option>
                    </select>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="row" style={{ marginTop: "0.75rem" }}>
        <button
          type="button"
          className="btn secondary"
          disabled={page <= 1}
          onClick={() => setPage((p) => p - 1)}
        >
          {t("common.previous")}
        </button>
        <button
          type="button"
          className="btn secondary"
          disabled={page >= totalPages}
          onClick={() => setPage((p) => p + 1)}
        >
          {t("patterns.next")}
        </button>
      </div>

      {confirmTest && (
        <div
          style={{
            position: "fixed",
            inset: 0,
            background: "rgba(0,0,0,0.6)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            zIndex: 100,
          }}
        >
          <div className="panel" style={{ maxWidth: 360 }}>
            <p>{t("patterns.mockConfirm")}</p>
            <p className="mono">{confirmTest.slice(0, 24)}…</p>
            <div className="row">
              <button
                type="button"
                className="btn"
                onClick={() => runMockTest(confirmTest)}
              >
                {t("patterns.mockRun")}
              </button>
              <button
                type="button"
                className="btn secondary"
                onClick={() => setConfirmTest(null)}
              >
                {t("common.cancel")}
              </button>
            </div>
          </div>
        </div>
      )}

      {toast && (
        <div className={`toast ${toast.kind === "error" ? "error" : "ok"}`}>
          {toast.text}
        </div>
      )}
    </div>
  );
}
