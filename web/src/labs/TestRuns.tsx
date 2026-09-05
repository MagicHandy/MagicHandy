import {useEffect,useRef,useState} from "react";
import {api} from "../api/client";
import {t,translateKnown} from "../i18n";
import {useAppState,useHashRoute} from "../state/app-state";
import {exportLabReport,labApi} from "./api";
import {FlowComparison} from "./FlowComparison";
import {openTestRun,testApi,type ReviewBasis,type TestRunList,type TestRunView,type TestStep} from "./test-api";

export const ratingLabel=(rating:number)=>rating===3?t("Good"):rating===2?t("Mixed"):rating===1?t("Needs work"):t("Skipped");
export const basisLabel=(basis:ReviewBasis)=>basis==="preview"?t("Visual preview"):basis==="device"?t("Device audition"):basis==="simulation"?t("Simulated audition"):basis==="reply"?t("LLM response"):t("Not tested");

export function TestRuns() {
  const id=useHashRoute().split("/")[3]||"";
  return id?<TestRunPage key={id} id={id}/>:<TestRunHome/>;
}

function TestRunHome() {
  const {backendOnline,readOnly}=useAppState();
  const [data,setData]=useState<TestRunList|null>(null);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [reload,setReload]=useState(0);
  useEffect(()=>{
    let live=true;
    void testApi.list().then(next=>{if(live){setData(next);setError("");}}).catch(reason=>{if(live)setError(String(reason));});
    return()=>{live=false;};
  },[reload]);
  async function create(experiments=false) {
    setBusy(true);setError("");
    try {openTestRun(await testApi.create(experiments?"motion_experiments":"motion_comparison"));}
    catch(reason) {setError(String(reason));}
    finally {setBusy(false);}
  }
  return <section className="lab-tests-page">
    <div className="lab-panel lab-test-welcome"><span className="lab-eyebrow">{t("Guided tests")}</span><h2>{t("Small rounds. Useful feedback.")}</h2>
      <p>{t("Follow a sequence, rate each result and add a comment. Every answer helps, including problems and skipped tests.")}</p>
      <button className="btn btn-primary" disabled={busy||readOnly||!backendOnline} onClick={()=>void create()}>{busy?t("Preparing tests…"):t("Start a motion feel check")}</button>
      <button className="btn btn-secondary" disabled={busy||readOnly||!backendOnline} onClick={()=>void create(true)}>{t("Compare motion experiments")}</button>
      <p className="hint">{t("Five rounds compare a continuous reference, correlated drift, softer reversals, a steadier beat and their combination.")}</p>
      <p className="hint">{t("Compare the available generators using the current lab score. Creating a sequence does not start motion.")}</p>
      <p className="hint">{t("For a specific change, choose Create test sequence beside a motion preview or an LLM reply.")}</p>
    </div>
    {error&&<p role="alert" className="form-status">{translateKnown(error)} <button className="btn btn-secondary" onClick={()=>setReload(value=>value+1)}>{t("Retry")}</button></p>}
    {!data&&!error&&<p role="status">{t("Loading…")}</p>}
    <h3>{t("Your sequences")}</h3>
    {data?.runs.length===0&&<p className="hint">{t("Your first sequence will appear here. Saved progress survives restarts.")}</p>}
    <div className="lab-test-list">{data?.runs.map(run=><a className="lab-panel lab-test-list-item" href={`#/labs/tests/${encodeURIComponent(run.id)}`} key={run.id}>
      <strong>{translateKnown(run.title)}</strong><span>{run.completed===run.total?t("Sequence complete"):t("{done} of {total} rounds saved",{done:run.completed,total:run.total})}</span><time dateTime={run.created_at}>{new Date(run.created_at).toLocaleString()}</time>
    </a>)}</div>
    <TestStorage path={data?.storage_path}/>
  </section>;
}

function TestRunPage({id}:{id:string}) {
  const {state,backendOnline,readOnly,motion,refresh}=useAppState();
  const [view,setView]=useState<TestRunView|null>(null);
  const [error,setError]=useState("");
  const [busy,setBusy]=useState(false);
  const [reload,setReload]=useState(0);
  const [confirmDelete,setConfirmDelete]=useState(false);
  const title=useRef<HTMLHeadingElement>(null);
  const limits=JSON.stringify(state?.settings?.motion);
  const locked=busy||readOnly||!backendOnline;
  const moving=Boolean(motion?.engine?.running||motion?.engine?.paused);
  useEffect(()=>{
    const controller=new AbortController();setError("");
    void testApi.get(id,controller.signal).then(next=>{if(!controller.signal.aborted)setView(current=>!current||next.run.revision>=current.run.revision?next:current);}).catch(reason=>{if(!controller.signal.aborted)setError(String(reason));});
    return()=>controller.abort();
  },[id,reload,limits]);
  useEffect(()=>{title.current?.focus();},[view?.next_index]);
  async function remove() {
    setBusy(true);setError("");
    try {await testApi.remove(id);window.location.hash="#/labs/tests";}
    catch(reason) {setError(String(reason));}
    finally {setBusy(false);}
  }
  async function stop() {
    setError("");
    try {await api.stopMotion();}
    catch(reason) {setError(String(reason));}
    finally {await refresh();}
  }
  const complete=view&&view.next_index===view.run.steps.length;
  return <section className="lab-tests-page">
    <div className="lab-panel-heading"><a href="#/labs/tests">{t("All sequences")}</a>{view&&<button className="btn btn-secondary" onClick={()=>exportLabReport(`lab-test-${view.run.id}.json`,{run:view.run,exported_at:new Date().toISOString()})}>{t("Export feedback")}</button>}</div>
    {error&&<p role="alert" className="form-status">{translateKnown(error)} <button className="btn btn-secondary" disabled={busy} onClick={()=>setReload(value=>value+1)}>{t("Reload sequence")}</button></p>}
    {!view&&!error&&<p role="status">{t("Loading…")}</p>}
    {view&&<>
      <header className="lab-test-progress"><span className="lab-eyebrow">{complete?t("Sequence complete"):t("Round {current} of {total}",{current:view.next_index+1,total:view.run.steps.length})}</span>
        <h2 ref={title} tabIndex={-1}>{translateKnown(view.run.title)}</h2>
        <progress aria-label={t("Saved test progress")} max={view.run.steps.length} value={view.next_index}/>
        <p className="hint" role="status">{t("{done} of {total} rounds saved",{done:view.next_index,total:view.run.steps.length})}</p>
      </header>
      {!complete?<TestRound key={view.run.steps[view.next_index].id} view={view} locked={locked} moving={moving} saveView={setView} setBusy={setBusy} setError={setError} stop={stop}/>:<div className="lab-panel lab-test-complete">
        <svg className="lab-test-check" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><path d="m5 12 4 4L19 6"/></svg><h3>{t("Feedback collected")}</h3>
        <p>{t("You finished the sequence. Review the results below or export them for the next iteration.")}</p>
        <p className="hint">{t("{reviewed} reviewed · {skipped} skipped",{reviewed:view.run.steps.filter(step=>step.feedback?.rating).length,skipped:view.run.steps.filter(step=>step.feedback?.rating===0).length})}</p>
      </div>}
      {view.next_index>0&&<details className="lab-panel lab-test-summary" open={Boolean(complete)}><summary>{t("Saved feedback")}</summary>
        {view.run.steps.filter(step=>step.feedback).map(step=><article key={step.id}><strong>{translateKnown(step.title)}</strong><p>{ratingLabel(step.feedback!.rating)} · {basisLabel(step.feedback!.basis)}</p><p className="lab-observation-text">{step.feedback!.comment||t("No comment")}</p><TestSource step={step}/></article>)}
      </details>}
      <TestStorage path={view.storage_path}/>
      <div className="row-actions lab-test-delete">{confirmDelete?<><span>{t("Delete this sequence and its feedback?")}</span><button className="btn btn-secondary" disabled={locked||moving} onClick={()=>void remove()}>{t("Confirm delete")}</button><button className="btn btn-secondary" onClick={()=>setConfirmDelete(false)}>{t("Cancel")}</button></>:<button className="lab-text-button" disabled={locked||moving} onClick={()=>setConfirmDelete(true)}>{t("Delete sequence")}</button>}</div>
    </>}
  </section>;
}

function TestRound({view,locked,moving,saveView,setBusy,setError,stop}:{view:TestRunView;locked:boolean;moving:boolean;saveView:(view:TestRunView)=>void;setBusy:(busy:boolean)=>void;setError:(error:string)=>void;stop:()=>Promise<void>}) {
  const {state,motion,refresh}=useAppState();
  const step=view.run.steps[view.next_index];
  const [rating,setRating]=useState<number|null>(null);
  const [basis,setBasis]=useState<ReviewBasis|"">("");
  const [comment,setComment]=useState("");
  async function save(skip=false) {
    setBusy(true);setError("");
    try {saveView(await testApi.feedback(view,skip?0:rating!,skip?"skipped":basis as ReviewBasis,comment));}
    catch(reason) {setError(String(reason));}
    finally {setBusy(false);}
  }
  async function audition() {
    if(!view.can_audition||!step.preview)return;
    setBusy(true);setError("");
    try {await labApi.start(step.preview,step.preview.candidates[0]);setBasis(state?.motion_simulated?"simulation":"device");}
    catch(reason) {setError(String(reason));}
    finally {await refresh();setBusy(false);}
  }
  return <div className="lab-test-round">
    <div className="lab-panel lab-test-stimulus"><h3>{translateKnown(step.title)}</h3><p>{translateKnown(step.instruction)}</p>
      {step.source.trial&&<div className="lab-test-reply"><p><strong>{t("Request")}: </strong>{step.source.trial.message}</p>{step.phase!=="before"&&<><p><strong>{t("Reply")}: </strong>{step.source.trial.reply||step.source.trial.error}</p>{!step.source.trial.valid&&<p className="form-status">{t("Rejected")}</p>}</>}</div>}
      {step.preview&&<FlowComparison preview={step.preview} selected={step.preview.candidates[0]} compact/>}
      {view.warning&&<p className="hint-block" role="status">{translateKnown(view.warning)}</p>}
      <div className="row-actions"><button className="btn btn-start" disabled={locked||!view.can_audition||motion?.available===false} onClick={()=>void audition()}>{state?.motion_simulated?t("Start simulated test"):t("Audition this round")}</button><button className="btn btn-secondary" onClick={()=>void stop()}>{t("Stop")}</button></div>
      <p className="hint">{t("Audition is optional and repeats until Stop. Stop motion before saving a round. The next round never starts automatically.")}</p>
      <TestSource step={step}/>
    </div>
    <form className="lab-panel lab-test-feedback" onSubmit={event=>{event.preventDefault();if(rating&&basis&&!locked&&!moving)void save();}}>
      <h3>{t("How was this round?")}</h3>
      <fieldset disabled={locked}><legend>{t("Your rating")}</legend><div className="lab-test-ratings">{[1,2,3].map(value=><label key={value} className={rating===value?"selected":""}><input type="radio" name={`rating-${step.id}`} checked={rating===value} onChange={()=>setRating(value)}/>{ratingLabel(value)}</label>)}</div></fieldset>
      <label className="field"><span className="label">{t("What did you review?")}</span><select value={basis} disabled={locked} onChange={event=>setBasis(event.target.value as ReviewBasis)}><option value="">{t("Choose a review basis")}</option>{([...(step.preview?["preview","device","simulation"]:[]),...(step.source.trial&&step.phase!=="before"?["reply"]:[])] as ReviewBasis[]).map(value=><option key={value} value={value}>{basisLabel(value)}</option>)}</select></label>
      <label className="field"><span className="label">{t("Comment for this test")}</span><textarea rows={4} maxLength={2000} value={comment} disabled={locked} placeholder={t("What worked, what felt wrong, or what should change?")} onChange={event=>setComment(event.target.value)}/></label>
      <p className="hint">{t("Comments are optional. Include the device and transport if relevant. Your review basis is self-reported.")}</p>
      {moving&&<p role="status" className="hint-block">{t("Stop motion to save feedback and continue.")}</p>}
      <button type="submit" className="btn btn-primary" disabled={locked||moving||!rating||!basis}>{view.next_index+1===view.run.steps.length?t("Save and finish"):t("Save and next")}</button>
      <button type="button" className="lab-text-button" disabled={locked||moving} onClick={()=>void save(true)}>{t("Skip this round")}</button>
      <p className="hint">{t("Save records the rating, comment and captured test together. Skipping also keeps your comment.")}</p>
    </form>
  </div>;
}

function TestSource({step}:{step:TestStep}) {
  return <details className="lab-score"><summary>{t("Captured source")}</summary><pre>{JSON.stringify({method:step.source.method,spec:step.source.spec,limits:step.source.settings,trial:step.source.trial},null,2)}</pre></details>;
}
function TestStorage({path}:{path?:string}) {
  return <details className="lab-storage"><summary>{t("Where feedback is saved")}</summary><p>{t("Sequences, captured previews and saved answers stay in this app’s database across restarts. Unsaved comments stay only on this page.")}</p>{path&&<code className="lab-storage-path">{path}</code>}<p>{t("Feedback is available for review and JSON export. It does not automatically change prompts or motion, or get sent to a model.")}</p></details>;
}
