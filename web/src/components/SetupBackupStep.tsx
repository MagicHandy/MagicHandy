import { t } from "../i18n";
import { DismissibleNotice } from "./DismissibleNotice";
import { SetupChoice } from "./SetupChoice";
import { SetupChatStep, type SetupChatStepProps } from "./SetupChatStep";

export function SetupBackupStep({ enabled, choose, ...local }: SetupChatStepProps & {
  enabled: boolean | null;
  choose: (enabled: boolean) => void;
}) {
  return <div className="setup-copy">
    <p>{t("Would you like to set up a local backup model?")}</p>
    <DismissibleNotice id="setup-local-backup" className="model-help"><p>{t("Your hosted model stays primary. A local backup can retry once when the provider explicitly declines a chat request. It does not take over for outages or usage limits.")}</p></DismissibleNotice>
    <div className="setup-choices" role="radiogroup" aria-label={t("Local backup (optional)")}>
      <SetupChoice selected={enabled === true} disabled={local.locked} title={t("Set up a local backup")} detail={t("Retries a declined chat once locally. Not used for outages, sign-in problems or usage limits.")} onSelect={() => choose(true)} />
      <SetupChoice selected={enabled === false} disabled={local.locked} title={t("Continue without a backup")} detail={t("Keep automatic local retry off. You can add a backup later in Settings > Chat > Model.")} onSelect={() => choose(false)} />
    </div>
    {enabled && <fieldset disabled={local.locked} className="setup-local-fields"><SetupChatStep {...local} localOnly /></fieldset>}
  </div>;
}
