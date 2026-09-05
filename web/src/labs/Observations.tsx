import {useEffect,useState} from "react";
import {t,translateKnown} from "../i18n";
import {useAppState} from "../state/app-state";
import {exportLabReport,labApi,type LabObservation,type LabObservations,type ObservationTarget} from "./api";
import {methodLabel} from "./FlowComparison";

export function ObservationEditor({target,label,close}:{target:ObservationTarget;label:string;close:()=>void}) {
  const {backendOnline,readOnly}=useAppState();
  const [text,setText]=useState("");
  const [busy,setBusy]=useState(false);
  const [saved,setSaved]=useState<LabObservations|null>(null);
  const [error,setError]=useState("");
  async function save() {
    setBusy(true);setError("");
    try {setSaved(await labApi.saveObservation(target,text.trim()));}
    catch(reason) {setError(reason instanceof Error?reason.message:t("Request failed"));}
    finally {setBusy(false);}
  }
  return <section className="lab-observation-editor" aria-label={t("Record an observation")}>
    <strong>{t("Observations")}</strong>
    <p className="hint">{t("Attached to: {source}",{source:label})}</p>
    {saved?<>
      <p role="status">{t("Observation saved.")} <a href="#/labs/observations">{t("View saved observations")}</a></p>
      <p className="hint">{t("Saved in this app’s local database:")} <code className="lab-storage-path">{saved.storage_path}</code></p>
    </>:<>
      <label className="field"><span className="label">{t("Your observation")}</span><textarea rows={3} maxLength={2000} value={text} disabled={busy||readOnly} onChange={event=>setText(event.target.value)}/></label>
      <p className="hint">{t("Save keeps this observation and its source in the app’s local database. It does not change prompts, preferences or motion. Use in chat copies it to a draft for you to send.")}</p>
      <p className="hint">{t("Describe whether you reviewed the plotted estimate or felt a device audition, and include the transport when relevant.")}</p>
      {error&&<p className="form-status" role="alert">{translateKnown(error)}</p>}
    </>}
    <div className="row-actions">
      {!saved&&<button className="btn btn-primary" disabled={!text.trim()||busy||readOnly||!backendOnline} onClick={()=>void save()}>{busy?t("Saving…"):t("Save observation")}</button>}
      <button className="btn btn-secondary" disabled={busy} onClick={close}>{saved?t("Done"):t("Cancel")}</button>
    </div>
  </section>;
}

export function observationDraft(row:LabObservation):string {
  const context=row.trial?row.trial.message:`${methodLabel(row.method)}; ${row.spec.min_percent}–${row.spec.max_percent}; ${row.spec.speed_percent}%`;
  // An editable next message; saved records never enter model context automatically.
  return t("Observation on {source}: {observation}",{source:context,observation:row.text});
}

export function ObservationsPage({useInChat}:{useInChat:(text:string)=>void}) {
  const {backendOnline,readOnly}=useAppState();
  const [data,setData]=useState<LabObservations|null>(null);
  const [error,setError]=useState("");
  const [removing,setRemoving]=useState("");
  const [confirmDelete,setConfirmDelete]=useState("");
  const [reload,setReload]=useState(0);
  useEffect(()=>{
    let live=true;setError("");
    void labApi.observations().then(next=>{if(live)setData(next);}).catch(reason=>{if(live)setError(String(reason));});
    return()=>{live=false;};
  },[reload]);
  async function remove(id:string) {
    setRemoving(id);setError("");
    try {setData(await labApi.deleteObservation(id));setConfirmDelete("");}
    catch(reason) {setError(String(reason));}
    finally {setRemoving("");}
  }
  return <section className="lab-observations-page">
    <div className="lab-panel-heading"><div><h2>{t("Saved observations")}</h2><p className="hint">{t("Review records attached to a motion preview or an LLM reply.")}</p></div>
      <button className="btn btn-secondary" disabled={!data?.observations.length} onClick={()=>exportLabReport("lab-observations.json",{observations:data?.observations,exported_at:new Date().toISOString()})}>{t("Export observations")}</button>
    </div>
    <details className="lab-storage" open><summary>{t("Storage and use")}</summary>
      <p>{t("Observations stay in this app’s local database across restarts and new lab chats. They are review evidence, not model training or automatic instructions.")}</p>
      {data&&<p><span>{t("Database")}</span> <code className="lab-storage-path">{data.storage_path}</code></p>}
      <p>{t("Use in chat opens an editable message. Only sending that message shares the observation with the selected LLM. Export creates a JSON file with the observation and its captured source.")}</p>
      <p className="hint">{t("Older unsaved observation fields were export-only; they cannot be recovered here.")}</p>
    </details>
    {error&&<p role="alert" className="form-status">{translateKnown(error)} <button className="btn btn-secondary" onClick={()=>setReload(value=>value+1)}>{t("Retry")}</button></p>}
    {!data&&!error&&<p role="status">{t("Loading…")}</p>}
    {data?.observations.length===0&&<div className="lab-empty"><h3>{t("No observations yet")}</h3><p>{t("Choose Observe preview in Motion Lab or Observe reply beside an LLM response, then save your observation.")}</p><a href="#/labs/chat">{t("Open LLM Lab")}</a></div>}
    <div className="lab-observations-list">{data?.observations.map(row=><article className="lab-observation" key={row.id}>
      <div className="lab-panel-heading"><strong>{row.source==="llm"?t("LLM reply"):methodLabel(row.method)}</strong><time dateTime={row.created_at}>{new Date(row.created_at).toLocaleString()}</time></div>
      <p className="lab-observation-text">{row.text}</p>
      <details className="lab-score"><summary>{t("Captured source")}</summary>
        {row.trial&&<><p><strong>{t("Request")}: </strong>{row.trial.message}</p><p><strong>{t("Reply")}: </strong>{row.trial.reply||row.trial.error}</p><p className="hint">{row.trial.model} · {row.method}</p></>}
        <pre>{JSON.stringify({spec:row.spec,limits:row.settings,trial:row.trial},null,2)}</pre>
      </details>
      <div className="row-actions"><button className="btn btn-secondary" disabled={readOnly} onClick={()=>useInChat(observationDraft(row))}>{t("Use in chat")}</button>
        {confirmDelete===row.id?<><span>{t("Delete this observation?")}</span><button className="btn btn-secondary" disabled={Boolean(removing)||readOnly||!backendOnline} onClick={()=>void remove(row.id)}>{t("Confirm delete")}</button><button className="btn btn-secondary" disabled={Boolean(removing)} onClick={()=>setConfirmDelete("")}>{t("Cancel")}</button></>
          :<button className="btn btn-secondary" disabled={Boolean(removing)||readOnly||!backendOnline} onClick={()=>setConfirmDelete(row.id)}>{t("Delete")}</button>}
      </div>
    </article>)}</div>
    {data&&data.observations.length>0&&<p className="hint">{t("{count} of {capacity} saved observations",{count:data.observations.length,capacity:data.capacity})}</p>}
  </section>;
}
