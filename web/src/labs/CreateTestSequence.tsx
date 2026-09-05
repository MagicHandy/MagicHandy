import {useState} from "react";
import {t,translateKnown} from "../i18n";
import {useAppState} from "../state/app-state";
import type {ObservationTarget} from "./api";
import {openTestRun,testApi} from "./test-api";

export function CreateTestSequence({target,disabled=false,experiments=false}:{target:ObservationTarget;disabled?:boolean;experiments?:boolean}) {
  const {readOnly,backendOnline}=useAppState();
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  async function create() {
    setBusy(true);setError("");
    try {openTestRun(await testApi.create(experiments?"motion_experiments":target.source==="llm"?"llm_comparison":"motion_comparison",target));}
    catch(reason) {setError(String(reason));}
    finally {setBusy(false);}
  }
  return <><button className="btn btn-secondary" disabled={disabled||busy||readOnly||!backendOnline} onClick={()=>void create()}>{busy?t("Preparing tests…"):experiments?t("Compare motion experiments"):t("Create test sequence")}</button>{error&&<p role="alert" className="form-status">{translateKnown(error)}</p>}</>;
}
