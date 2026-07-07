import { useTranslation } from "react-i18next";

export function ConnectionPill({
  label,
  connected,
  detail,
  onClick,
}: {
  label: string;
  connected: boolean;
  detail?: string | null;
  onClick?: () => void;
}) {
  const { t } = useTranslation();

  return (
    <button
      type="button"
      className={`conn-pill${connected ? " on" : " off"}`}
      title={
        detail
          ? t("device.pill.detailTitle", { detail })
          : t("device.pill.refreshTitle")
      }
      onClick={onClick}
      disabled={!onClick}
    >
      <span className="conn-dot" />
      <span className="conn-label">{label}</span>
      <span className="conn-state">{connected ? t("common.ok") : t("common.off")}</span>
    </button>
  );
}
