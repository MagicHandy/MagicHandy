import { useTranslation } from "react-i18next";
import type { ControllerSnapshot } from "../api/types";

export function ControllerBanner({ controller }: { controller: ControllerSnapshot | null }) {
  const { t } = useTranslation();
  if (!controller?.read_only) return null;
  return (
    <div className="alert alert-warn controller-banner" role="status">
      <strong>{t("layout.controller.readOnly")}</strong>
      {controller.reason ? <span> — {controller.reason}</span> : null}
    </div>
  );
}
