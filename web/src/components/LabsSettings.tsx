import {useState} from "react";
import {api} from "../api/client";
import {t,translateKnown} from "../i18n";
import {useAppState} from "../state/app-state";

export function LabsSettings() {
  const {state,backendOnline,readOnly,refresh}=useAppState();
  const [pending,setPending]=useState(false);
  const [error,setError]=useState("");
  async function change(enabled:boolean) {
    setPending(true);setError("");
    try {await api.setLabsEnabled(enabled);}
    catch(reason) {setError(reason instanceof Error?reason.message:t("Request failed"));}
    finally {await refresh();setPending(false);}
  }
  return <div className="group">
    <h3 className="group-title">{t("Labs")}</h3>
    <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={Boolean(state?.labs_enabled)} disabled={pending||readOnly||!backendOnline} onChange={event=>void change(event.target.checked)}/><span className="track" aria-hidden="true"/></span><span>{t("Enable Labs")}</span></label>
    <p className="hint-block">{t("Show the experimental motion, LLM chat and observation workspace in the sidebar. Available in every release; applies immediately and survives restarts.")}</p>
    <p className="hint-block">{t("Disabling Labs cancels lab requests and stops lab auditions. Saved observations remain.")}</p>
    {state?.labs_enabled&&<a className="btn btn-secondary" href="#/labs/chat">{t("Open Labs")}</a>}
    {error&&<p role="alert" className="form-status">{translateKnown(error)}</p>}
  </div>;
}
