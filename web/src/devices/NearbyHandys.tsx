// Native Bluetooth discovery for one Handy, primary or additional: nearby
// Handys, strongest signal first, so the right one is chosen when several
// advertise. Candidates are opaque tokens; one another connection holds is
// listed but cannot be chosen.
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { HandyCandidate } from "../api/handy-types";
import type { HandyInstanceId } from "../api/types";
import { SetupChoice } from "../components/SetupChoice";
import { t } from "../i18n";
import { OptionRow } from "./fields";
import { lockReason } from "./status";
import type { DeviceContext } from "./types";

export function NearbyHandys({ ctx, device, connected }: { ctx: DeviceContext; device: "handy" | HandyInstanceId; connected: boolean }) {
  const [candidates, setCandidates] = useState<HandyCandidate[] | null>(null);
  const [chosen, setChosen] = useState("");
  const busy = ctx.busy[device];
  // Discovery opens Bluetooth on the host, like Connect.
  const reason = lockReason(ctx, true);
  const unavailable = Boolean(reason) || Boolean(busy);

  useEffect(() => {
    if (!connected) return;
    setCandidates(null);
    setChosen("");
  }, [connected]);

  function discover() {
    setCandidates(null);
    setChosen("");
    void ctx.run(device, "discover", async () => {
      const found = (await api.handyDiscover(device)).candidates;
      setCandidates([...found].sort((a, b) => b.rssi - a.rssi));
    });
  }

  function connect() {
    void ctx.run(device, "connect", async () => {
      const link = await api.handyConnect(device, chosen);
      if (device !== "handy") ctx.setHandy(link);
    });
  }

  return (
    <>
      <OptionRow label={t("Find nearby Handys")} hint={t("Put the Handy in Bluetooth mode first. Use this when several Handys are nearby.")}>
        <button type="button" className="btn btn-secondary" title={reason} disabled={unavailable || connected} onClick={discover}>
          {busy === "discover" ? t("Finding devices…") : t("Find nearby Handys")}
        </button>
      </OptionRow>
      {candidates !== null && !connected && (
        <div className="form-row form-row-plain">
          {candidates.length ? (
            <>
              <div className="setup-choices" role="radiogroup" aria-label={t("Nearby Handys")}>
                {candidates.map((candidate, index) => (
                  <SetupChoice key={candidate.id} selected={chosen === candidate.id}
                    title={candidate.name || t("Nearby Handy {number}", { number: index + 1 })}
                    detail={[t("{signal} dBm", { signal: candidate.rssi }), ...(candidate.in_use ? [t("already in use")] : [])].join(" · ")}
                    disabled={candidate.in_use || unavailable} onSelect={() => setChosen(candidate.id)} />
                ))}
              </div>
              <div className="row-actions">
                <button type="button" className="btn btn-primary" disabled={!chosen || unavailable} onClick={connect}>{t("Connect")}</button>
                <span className="hint">{t("Strongest signal first. A Handy already in use cannot be chosen.")}</span>
              </div>
            </>
          ) : <p className="hint-block">{t("No nearby Handys found. Put the Handy in Bluetooth mode and try again.")}</p>}
        </div>
      )}
    </>
  );
}
