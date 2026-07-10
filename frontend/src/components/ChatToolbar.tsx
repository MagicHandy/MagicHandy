import { useTranslation } from "react-i18next";
import type { OperationMode, StatusSnapshot } from "../api/types";

export function ChatToolbar({
  mode,
  onModeChange,
  disabled,
}: {
  snap?: StatusSnapshot | null;
  mode: OperationMode;
  onModeChange: (mode: OperationMode) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();

  return (
    <div className="chat-toolbar" role="toolbar" aria-label={t("chat.toolbarAria")}>
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
