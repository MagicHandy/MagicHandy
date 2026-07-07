import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { OperationMode, StatusSnapshot } from "../api/types";
import { useToast } from "../contexts/ToastContext";

export function ChatToolbar({
  snap,
  mode,
  onModeChange,
  disabled,
}: {
  snap: StatusSnapshot | null;
  mode: OperationMode;
  onModeChange: (mode: OperationMode) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const { notify } = useToast();

  const toggleAutospeak = async () => {
    if (!snap) return;
    try {
      const next = !snap.autospeak_enabled;
      await api.toggleAutospeak(next);
      notify(next ? t("chat.autospeakOn") : t("chat.autospeakOff"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <div className="chat-toolbar" role="toolbar" aria-label={t("chat.toolbarAria")}>
      <button
        type="button"
        className={`chat-tool-btn autospeak-toggle${snap?.autospeak_enabled ? " is-on" : ""}`}
        disabled={disabled}
        onClick={toggleAutospeak}
        title={
          snap?.autospeak_enabled
            ? t("chat.autospeakOnTitle")
            : t("chat.autospeakOffTitle")
        }
      >
        <span className="chat-tool-icon" aria-hidden>
          ◎
        </span>
        {snap?.autospeak_enabled ? t("chat.autospeakOnLabel") : t("chat.autospeakOffLabel")}
      </button>

      <label className="chat-tool-select">
        <span className="section-label">{t("chat.mode")}</span>
        <select
          className="select-mode"
          value={mode}
          disabled={disabled}
          onChange={(e) => onModeChange(e.target.value as OperationMode)}
        >
          <option value="manual">{t("chat.modeManual")}</option>
          <option value="auto">{t("chat.modeAuto")}</option>
          <option value="hybrid">{t("chat.modeHybrid")}</option>
        </select>
      </label>

      <span className="chat-toolbar-hint">{t("chat.ctrlEnter")}</span>
    </div>
  );
}
