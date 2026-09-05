import {t} from "../i18n";
import {lazy,Suspense} from "react";
import {useAppState} from "../state/app-state";
import {WorkspaceHead} from "../components/WorkspaceHead";

const Workspace=lazy(()=>import("./LabsRoute").then(module=>({default:module.LabsRoute})));
export function LabsRoute() {
  const {state}=useAppState();
  if(!state?.labs_enabled)return <section><WorkspaceHead title={t("Labs")}/><p>{t("Labs is disabled. Enable it in Settings > General.")}</p><a className="btn btn-secondary" href="#/settings/general">{t("Open Settings")}</a></section>;
  return <Suspense fallback={<p role="status">{t("Loading…")}</p>}><Workspace/></Suspense>;
}

export const LAB_BASE = "labs";
export function legacyLabRoute(hash:string) {
  const route=hash.replace(/^#\/?/, "");
  return route==="settings/motion-lab" ? "#/labs/motion" : route==="settings/llm-lab" ? "#/labs/chat" : "";
}
export function LabsNavLink({active,enabled}:{active:string;enabled:boolean}) {
  if(!enabled)return null;
  return <a className="nav-link" href="#/labs/chat" aria-label={t("Labs")} aria-current={active===LAB_BASE?"page":undefined}>
    <span className="icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M9 3h6M10 3v6l-6 10a1.3 1.3 0 0 0 1 2h14a1.3 1.3 0 0 0 1-2L14 9V3M7 15h10"/></svg></span>
    <span className="label">{t("Labs")}</span>
  </a>;
}
