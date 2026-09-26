import {useEffect,useRef,useState} from "react";
import {api} from "../api/client";
import {t,translateKnown} from "../i18n";
import {useAppState,useToast,useMotionState} from "../state/app-state";
import {exportLabReport,type LabTrial,type ObservationTarget} from "./api";
import {FlowComparison} from "./FlowComparison";
import {ObservationEditor} from "./Observations";
import {mainLabModes,useLLMLab,type ModeComparison} from "./useLLMLab";
import {CreateTestSequence} from "./CreateTestSequence";
import {LabHelpLink} from "./LabHelp";

const moreModes=["edits","controls","sequence","layers","library_actions","library_descriptive","library"];

function modeLabel(method:string):string {
  switch(method) {
  case "creative_v2":return t("Creative v2");
  case "layered":return t("Layered");
  case "stroke_ends":return t("Stroke ends");
  case "groove":return t("Groove and accents");
  case "plain_words":return t("Plain words");
  case "edits":return t("Relative and layer edits");
  case "controls":return t("Single controls");
  case "sequence":return t("Sequence");
  case "layers":return t("Layers");
  case "library_actions":return t("Library · action names");
  case "library_descriptive":return t("Library · descriptive IDs");
  case "library":return t("Library · opaque handles");
  default:return method;
  }
}

const quickRequests=()=>[t("Deeper"),t("Not so deep"),t("Just the tip"),t("Whole length"),t("Faster"),t("Slower"),t("Keep it like this"),t("Surprise me")];

function turnStatus(turn:LabTrial):[string,string] {
  if(turn.motion_applied)return ["applied",t("Motion updated")];
  if(!turn.valid)return ["rejected",t("Rejected")];
  return turn.changed.length?["changed",t("Preview updated")]:["same",t("No changes")];
}

// A compiled position trace, base at the bottom and tip at the top.
function Trace({values}:{values:number[]}) {
  if(values.length<2)return null;
  const points=values.map((value,index)=>`${(index/(values.length-1)*200).toFixed(1)},${(100-value).toFixed(1)}`).join(" ");
  return <svg className="lab-trace" viewBox="0 0 200 100" preserveAspectRatio="none" aria-hidden="true"><polyline points={points}/></svg>;
}

const ResendIcon=()=><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M13 8a5 5 0 1 1-1.46-3.54M13.5 2.5v3h-3" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg>;

function ComparisonPanel({comparison,current,locked,choose,close,cancel}:{comparison:ModeComparison;current:string;locked:boolean;choose:(method:string)=>void;close:()=>void;cancel:()=>void}) {
  const done=comparison.results.filter(item=>item.result||item.error).length;
  return <section className="lab-compare" aria-label={t("Mode comparison")}>
    <div className="lab-compare-head"><h2>{t("Mode comparison")}</h2>
      {comparison.running?<button type="button" className="lab-text-button" onClick={cancel}>{t("Cancel generation")}</button>:<button type="button" className="lab-text-button" onClick={close}>{t("Close")}</button>}</div>
    <p className="hint">{comparison.running?t("Comparing {current} of {total}…",{current:Math.min(done+1,comparison.results.length),total:comparison.results.length}):t("The same request in each mode, from its starting score. Nothing is saved or played.")}</p>
    <p className="lab-compare-message">{comparison.message}</p>
    <ol className="lab-compare-list">{comparison.results.map(({method,result,error})=>{
      const trial=result?.trial;
      const status=trial?turnStatus(trial):null;
      return <li key={method}>
        <div className="lab-compare-row"><strong>{modeLabel(method)}</strong>
          {status&&<span className={`lab-badge is-${status[0]}`}>{status[1]}</span>}
          {result&&trial?.valid&&<span className="hint">{t("Reach {min}–{max}",{min:Math.round(result.perceptual.position_min_percent),max:Math.round(result.perceptual.position_max_percent)})}</span>}
          <button type="button" className="lab-text-button" disabled={locked||method===current} onClick={()=>choose(method)}>{t("Try this mode")}</button>
        </div>
        {result?<><Trace values={result.trace}/><p className="lab-compare-reply">{trial?.reply||trial?.error}</p></>
          :error?<p className="form-status">{translateKnown(error)}</p>:comparison.running&&<p className="hint">{t("Generating…")}</p>}
      </li>;
    })}</ol>
  </section>;
}

export function LLMLab({initialDraft="",draftUsed=()=>{}}:{initialDraft?:string;draftUsed?:()=>void}) {
  const {state:app,refresh}=useAppState();
  const motion=useMotionState();
  const {show}=useToast();
  const lab=useLLMLab();
  const [message,setMessage]=useState("");
  const [configure,setConfigure]=useState(false);
  const [details,setDetails]=useState<number|null>(null);
  const [observed,setObserved]=useState<{target:ObservationTarget;label:string;index:number}|null>(null);
  const log=useRef<HTMLDivElement>(null);
  const composer=useRef<HTMLTextAreaElement>(null);
  const {state,preview,comparison}=lab;
  const session=state?.session;
  const testing=Boolean(session?.active);
  const live=Boolean(testing&&session?.live);
  const autopilot=Boolean(testing&&session?.autopilot);
  const configLocked=lab.locked||testing;
  const canSend=!lab.locked&&Boolean(lab.prompt.trim());
  const lastMessage=[...(state?.turns??[])].reverse().find(turn=>!turn.autopilot)?.message??"";
  const compareText=message.trim()||lastMessage;
  const generating=lab.busy&&(Boolean(lab.pendingMessage)||Boolean(comparison?.running));
  useEffect(()=>{if(initialDraft){setMessage(initialDraft);draftUsed();composer.current?.focus();}},[initialDraft,draftUsed]);
  useEffect(()=>{if(log.current)log.current.scrollTop=log.current.scrollHeight;},[state?.revision,lab.pendingMessage]);
  async function sendDraft() {if(await lab.send(message.trim())){setMessage("");composer.current?.focus();}}
  async function stop() {try{await api.stopMotion();}catch(reason){show(String(reason),"error");}finally{refresh();}}
  const candidate=preview?.candidates.find(item=>item.flow);
  return <div className="llm-lab-workspace lab-quick">
    <div className="lab-quick-bar">
      <div className="lab-modes" role="group" aria-label={t("Test mode")}>
        {mainLabModes.map(mode=><button key={mode} type="button" aria-pressed={lab.method===mode} disabled={lab.locked} onClick={()=>void lab.chooseMethod(mode)}>{modeLabel(mode)}</button>)}
        <select aria-label={t("More modes")} value={moreModes.includes(lab.method)?lab.method:""} disabled={lab.locked} onChange={event=>void lab.chooseMethod(event.target.value)}>
          <option value="" disabled>{t("More modes")}</option>
          {moreModes.map(mode=><option key={mode} value={mode}>{modeLabel(mode)}</option>)}
        </select>
      </div>
      <div className="lab-session-controls">
        <label className="lab-switch"><input type="checkbox" role="switch" checked={live} disabled={lab.locked||(!live&&motion?.available===false)} onChange={event=>void lab.setSession(event.target.checked,autopilot)}/>{t("Live motion")}</label>
        <label className="lab-switch"><input type="checkbox" role="switch" checked={autopilot} disabled={lab.locked} onChange={event=>void lab.setSession(live,event.target.checked)}/>{t("Autopilot")}</label>
        <button type="button" className="btn btn-secondary" onClick={()=>void stop()}>{t("Stop")}</button>
        <button type="button" className="lab-text-button" disabled={configLocked||!state?.turns.length} onClick={()=>{setObserved(null);setDetails(null);void lab.reset();}}>{t("New chat")}</button>
        <button type="button" className="lab-text-button" aria-expanded={configure} onClick={()=>setConfigure(value=>!value)}>{t("Configure")}</button>
        <LabHelpLink section="modes"/>
      </div>
    </div>
    {(testing||app?.motion_simulated||session?.error)&&<div className="lab-status-line">
      {testing&&<span role="status">{live?t("Live test running"):t("Preview test running")}</span>}
      {app?.motion_simulated&&<span className="hint">{t("Simulation active · device will not move")}</span>}
      {session?.error&&<span role="alert" className="form-status">{translateKnown(session.error)}</span>}
    </div>}
    {configure&&<div className="lab-chat-config">
      <label className="field"><span className="label">{t("Model")}</span><input value={lab.model} disabled={configLocked} onChange={event=>lab.setModel(event.target.value)}/></label>
      <label className="field"><span className="label">{t("Autopilot interval (seconds)")}</span><input type="number" min={5} max={120} value={lab.interval} disabled={configLocked} onChange={event=>lab.setInterval(Number(event.target.value))}/></label>
      <label><input type="checkbox" checked={lab.schemaGuided} disabled={configLocked} onChange={event=>lab.setSchemaGuided(event.target.checked)}/>{t("Constrain output schema")}</label>
      <div className="lab-config-links"><button type="button" className="lab-text-button" disabled={!state?.turns.length} onClick={()=>exportLabReport("llm-lab-trials.json",{...state,motion_simulated:app?.motion_simulated})}>{t("Export conversation")}</button><LabHelpLink section="conversation"/><LabHelpLink section="autopilot"/></div>
      <details className="lab-score"><summary>{t("Experimental prompt")}</summary><textarea aria-label={t("Experimental prompt")} rows={9} maxLength={16000} spellCheck={false} value={lab.prompt} disabled={configLocked} onChange={event=>lab.setPrompt(event.target.value)}/></details>
    </div>}
    <div className="lab-quick-main">
      <section className="lab-chat" aria-label={t("LLM Lab conversation")}>
        <div className="lab-chat-log" ref={log} role="log" aria-label={t("Lab messages")} aria-live="polite" aria-relevant="additions text">
          {!state&&!lab.error&&<p role="status" className="hint">{t("Loading…")}</p>}
          {state?.turns.length===0&&!lab.pendingMessage&&<p className="lab-chat-empty">{t("Pick a mode, then type a request or tap a quick one.")}</p>}
          {state?.turns.map((turn,index)=>{
            const [kind,label]=turnStatus(turn);
            const open=details===index;
            return <div className="lab-turn" key={`${state.revision-state.turns.length+index}:${turn.message}`}>
              {!turn.autopilot&&<div className="lab-user"><p>{turn.message}</p>
                <button type="button" className="lab-icon-button" aria-label={t("Send again")} title={t("Send again")} disabled={!canSend} onClick={()=>void lab.send(turn.message)}><ResendIcon/></button></div>}
              <div className="lab-reply">
                <div className="lab-reply-head"><span className={`lab-badge is-${kind}`}>{label}</span><span className="hint">{turn.autopilot?t("Autopilot"):modeLabel(turn.method)}</span>
                  <button type="button" className="lab-text-button" aria-expanded={open} onClick={()=>setDetails(open?null:index)}>{t("Details")}</button></div>
                <p className="lab-reply-text">{turn.reply||turn.error}</p>
                {turn.error&&turn.reply&&<p className="form-status">{turn.error}</p>}
                {turn.motion_error&&<p className="form-status" role="alert">{turn.motion_error}</p>}
                {open&&<div className="lab-reply-details">
                  <p className="hint">{t("{model} · {time} ms · {calls} provider calls",{model:turn.model,time:turn.elapsed_ms,calls:turn.provider_calls})}</p>
                  <p>{turn.recipe_name||turn.changed.join(", ")||t("No changes")}</p><pre>{turn.raw}</pre>
                  <div className="lab-reply-tools"><button type="button" className="lab-text-button" disabled={lab.locked} onClick={()=>setObserved({index,label:turn.message,target:{source:"llm",settings_key:state.settings_key,revision:state.revision,turn_index:index}})}>{t("Observe reply")}</button><LabHelpLink section="storage"/>
                    <CreateTestSequence disabled={lab.locked} target={{source:"llm",settings_key:state.settings_key,revision:state.revision,turn_index:index}}/></div>
                </div>}
                {observed?.index===index&&<ObservationEditor key={`${observed.target.revision}:${index}`} target={observed.target} label={observed.label} close={()=>setObserved(null)}/>}
              </div>
            </div>;
          })}
          {lab.pendingMessage&&<div className="lab-turn"><div className="lab-user"><p>{lab.pendingMessage}</p></div><p role="status" className="hint">{t("Generating…")}</p></div>}
          {state?.busy&&!lab.busy&&<p role="status" className="hint">{session?.autopilot?t("Autopilot is thinking…"):t("Another client is generating a lab reply.")}</p>}
        </div>
        <form className="lab-composer" onSubmit={event=>{event.preventDefault();void sendDraft();}}>
          {lab.error&&<p role="alert" className="form-status">{translateKnown(lab.error)} {!state&&<button type="button" className="lab-text-button" onClick={lab.retry}>{t("Retry")}</button>}</p>}
          <div className="lab-quick-requests" role="group" aria-label={t("Quick requests")}>
            {quickRequests().map(text=><button key={text} type="button" disabled={!canSend} onClick={()=>void lab.send(text)}>{text}</button>)}
          </div>
          <div className="lab-compose-row">
            <textarea ref={composer} aria-label={t("Message")} placeholder={t("Describe a change or ask a question…")} rows={2} maxLength={2000} value={message} disabled={lab.locked} onChange={event=>setMessage(event.target.value)}
              onKeyDown={event=>{if(event.key==="Enter"&&!event.shiftKey&&!event.nativeEvent.isComposing&&event.keyCode!==229){event.preventDefault();void sendDraft();}}}/>
            <div className="lab-compose-actions">
              {generating?<button type="button" className="btn btn-secondary" onClick={lab.cancel}>{t("Cancel generation")}</button>:<>
                <button type="submit" className="btn btn-primary" disabled={!canSend||!message.trim()}>{t("Send")}</button>
                <button type="button" className="btn btn-secondary" disabled={lab.locked||testing||!compareText.trim()} onClick={()=>void lab.compare(compareText)}>{t("Compare modes")}</button>
              </>}
            </div>
          </div>
          <p className="hint">{t("Enter to send · Shift+Enter for a new line")}</p>
        </form>
      </section>
      <aside className="lab-motion" aria-label={t("Motion now")}>
        {comparison&&<ComparisonPanel comparison={comparison} current={lab.method} locked={lab.locked} choose={method=>void lab.chooseMethod(method)} close={lab.closeComparison} cancel={lab.cancel}/>}
        <section className="lab-motion-now"><h2>{t("Motion now")}</h2>
          {preview&&candidate?<FlowComparison preview={preview} selected={candidate} compact/>:<p className="hint">{t("Loading…")}</p>}
          <details className="lab-score"><summary>{t("Current score")}</summary><pre>{JSON.stringify(state?.current,null,2)}</pre></details>
        </section>
      </aside>
    </div>
  </div>;
}
