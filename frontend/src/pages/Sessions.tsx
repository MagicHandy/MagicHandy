import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, downloadBlob } from "../api/client";
import type { SessionRow } from "../api/types";
import { useToast } from "../contexts/ToastContext";

type SessionDetail = {
  session: SessionRow;
  persona?: { id: string; name: string } | null;
  messages: { role: string; content: string; created_at: string }[];
  events: { event_type: string; payload: unknown; created_at: string }[];
  blocks_played: {
    id: string;
    zone?: string;
    speed?: string;
    bpm?: number;
    duration_ms?: number;
  }[];
};

function roleLabel(role: string, t: (key: string) => string) {
  if (role === "assistant") return t("chat.roleAssistant");
  if (role === "user") return t("chat.roleUser");
  if (role === "system") return t("chat.roleSystem");
  return role;
}

function modeLabel(mode: string | null | undefined, t: (key: string) => string) {
  if (mode === "manual") return t("chat.modeManual");
  if (mode === "auto") return t("chat.modeAuto");
  if (mode === "hybrid") return t("chat.modeHybrid");
  return mode ?? "—";
}

export function SessionsPanel() {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [sessions, setSessions] = useState<SessionRow[]>([]);
  const [ratings, setRatings] = useState<Record<string, number>>({});
  const [notes, setNotes] = useState<Record<string, string>>({});
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<SessionDetail | null>(null);
  const [loadingDetail, setLoadingDetail] = useState(false);

  const load = async () => {
    const data = await api.listSessions();
    setSessions(data.sessions);
    const r: Record<string, number> = {};
    const n: Record<string, string> = {};
    data.sessions.forEach((s) => {
      if (s.rating != null) r[s.id] = s.rating;
      if (s.notes) n[s.id] = s.notes;
    });
    setRatings(r);
    setNotes(n);
  };

  useEffect(() => {
    load().catch((e) =>
      notify(e instanceof Error ? e.message : t("common.error"), "error"),
    );
  }, [notify, t]);

  const openDetail = async (id: string) => {
    setSelectedId(id);
    setLoadingDetail(true);
    try {
      const d = await api.getSession(id);
      setDetail(d as SessionDetail);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
      setDetail(null);
    } finally {
      setLoadingDetail(false);
    }
  };

  const exportSession = async (id: string) => {
    try {
      const { filename, blob } = await api.exportSession(id);
      downloadBlob(filename, blob);
      notify(t("sessions.exported", { filename }), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const submitRating = async (id: string) => {
    const rating = ratings[id];
    if (!rating || rating < 1 || rating > 5) {
      notify(t("sessions.ratingRequired"), "error");
      return;
    }
    try {
      await api.sessionFeedback(id, rating, notes[id]);
      notify(t("sessions.feedbackSaved"), "ok");
      await load();
      if (selectedId === id) await openDetail(id);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <div className="sessions-layout">
      <div className="glass table-panel">
        {sessions.length === 0 ? (
          <p className="hint center">{t("sessions.empty")}</p>
        ) : (
          <div className="table-scroll">
            <table className="data-table">
              <thead>
                <tr>
                  <th>{t("sessions.colStart")}</th>
                  <th>{t("sessions.colMode")}</th>
                  <th>{t("sessions.colRating")}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {sessions.map((s) => (
                  <tr
                    key={s.id}
                    className={selectedId === s.id ? "row-selected" : ""}
                  >
                    <td className="mono">{s.started_at?.slice(0, 19) ?? "—"}</td>
                    <td>{modeLabel(s.mode, t)}</td>
                    <td>{s.rating ?? "—"}</td>
                    <td>
                      <button
                        type="button"
                        className="btn btn-ghost btn-sm"
                        onClick={() => openDetail(s.id)}
                      >
                        {t("sessions.replay")}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {selectedId && (
        <div className="glass session-detail-panel">
          {loadingDetail || !detail ? (
            <p className="hint center">{t("sessions.loadingReplay")}</p>
          ) : (
            <>
              <div className="session-detail-head">
                <h3>{t("sessions.replayTitle")}</h3>
                <div className="btn-row">
                  <button
                    type="button"
                    className="btn btn-sm btn-primary"
                    onClick={() => exportSession(selectedId)}
                  >
                    {t("sessions.exportZip")}
                  </button>
                  <button
                    type="button"
                    className="btn btn-sm btn-ghost"
                    onClick={() => {
                      setSelectedId(null);
                      setDetail(null);
                    }}
                  >
                    {t("sessions.close")}
                  </button>
                </div>
              </div>
              <p className="hint mono">
                {detail.session.started_at?.slice(0, 19)} · {modeLabel(detail.session.mode, t)} ·{" "}
                {detail.persona?.name ?? detail.session.persona_id ?? "—"}
              </p>

              <h4 className="section-label">{t("sessions.chatSection")}</h4>
              <div className="session-chat-replay">
                {detail.messages.length === 0 ? (
                  <p className="hint">{t("sessions.noMessages")}</p>
                ) : (
                  detail.messages.map((m, i) => (
                    <div
                      key={i}
                      className={`session-chat-line session-chat-line--${m.role}`}
                    >
                      <span className="mono">{roleLabel(m.role, t)}</span>
                      <p>{m.content}</p>
                    </div>
                  ))
                )}
              </div>

              <h4 className="section-label">{t("sessions.blocksPlayed")}</h4>
              {detail.blocks_played.length === 0 ? (
                <p className="hint">{t("sessions.noBlocks")}</p>
              ) : (
                <ul className="queue-list">
                  {detail.blocks_played.map((b) => (
                    <li key={b.id}>
                      <span className="mono">{b.id.slice(0, 18)}…</span>
                      <span>
                        {b.zone} · {b.speed}
                        {b.bpm != null
                          ? t("sessions.bpmSuffix", { bpm: Math.round(b.bpm) })
                          : ""}
                        {b.duration_ms != null
                          ? t("sessions.durationSuffix", {
                              sec: (b.duration_ms / 1000).toFixed(1),
                            })
                          : ""}
                      </span>
                    </li>
                  ))}
                </ul>
              )}

              <h4 className="section-label">{t("sessions.rateTitle")}</h4>
              <div className="form-grid two">
                <label className="field">
                  <span>{t("sessions.ratingLabel")}</span>
                  <input
                    type="number"
                    min={1}
                    max={5}
                    value={ratings[selectedId] ?? detail.session.rating ?? ""}
                    onChange={(e) =>
                      setRatings({
                        ...ratings,
                        [selectedId]: Number(e.target.value),
                      })
                    }
                  />
                </label>
                <label className="field">
                  <span>{t("sessions.notes")}</span>
                  <input
                    value={notes[selectedId] ?? detail.session.notes ?? ""}
                    onChange={(e) =>
                      setNotes({ ...notes, [selectedId]: e.target.value })
                    }
                  />
                </label>
              </div>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => submitRating(selectedId)}
              >
                {t("sessions.saveFeedback")}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}
