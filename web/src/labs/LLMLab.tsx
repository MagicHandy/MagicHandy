import {useEffect,useRef,useState} from "react";
import {api} from "../api/client";
import {t,translateKnown} from "../i18n";
import {useAppState,useToast} from "../state/app-state";
import {exportLabReport,type ObservationTarget} from "./api";
import {FlowComparison} from "./FlowComparison";
import {ObservationEditor} from "./Observations";
import {useLLMLab} from "./useLLMLab";
import {CreateTestSequence} from "./CreateTestSequence";
import {LabHelpLink} from "./LabHelp";

export function LLMLab({initialDraft="",draftUsed=()=>{}}:{initialDraft?:string;draftUsed?:()=>void}) {
  const {state:app,motion,refresh}=useAppState();
  const {show}=useToast();
  const lab=useLLMLab();
  const [message,setMessage]=useState("");
  const [configure,setConfigure]=useState(false);
  const [live,setLive]=useState(false);
  const [autopilot,setAutopilot]=useState(false);
  const [interval,setInterval]=useState(20);
  const [observed,setObserved]=useState<{target:ObservationTarget;label:string;index:number}|null>(null);
  const log=useRef<HTMLDivElement>(null);
  const composer=useRef<HTMLTextAreaElement>(null);
  const {state,preview}=lab;
  const session=state?.session;
  const testing=Boolean(session?.active);
  const configLocked=lab.locked||testing;
  useEffect(()=>{if(initialDraft){setMessage(initialDraft);draftUsed();composer.current?.focus();}},[initialDraft,draftUsed]);
  useEffect(()=>{if(log.current)log.current.scrollTop=log.current.scrollHeight;},[state?.revision,lab.pendingMessage]);
  async function send() {if(await lab.send(message.trim())){setMessage("");composer.current?.focus();}}
  async function stop() {try{await api.stopMotion();}catch(reason){show(String(reason),"error");}finally{refresh();}}
  const candidate=preview?.candidates.find(item=>item.flow);
  const modes=[["edits",t("Relative and layer edits")],["controls",t("Single controls")],["sequence",t("Sequence")],["layers",t("Layers")],["library_actions",t("Library · action names")],["library_descriptive",t("Library · descriptive IDs")],["library",t("Library · opaque handles")]];
  return <div className="llm-lab-workspace">
    <section className="lab-chat" aria-label={t("LLM Lab conversation")}>
      <div className="lab-chat-toolbar">
        <strong>{t("LLM Lab")}</strong>
        <label className="lab-mode-picker"><span>{t("Test mode")}</span><select aria-label={t("Test mode")} value={lab.method} disabled={configLocked} onChange={event=>lab.chooseMethod(event.target.value)}>{modes.map(([id,label])=><option key={id} value={id}>{label}</option>)}</select></label>
        <LabHelpLink section="modes"/><button className="lab-text-button" aria-expanded={configure} onClick={()=>setConfigure(value=>!value)}>{t("Configure")}</button>
        <button className="lab-text-button" disabled={configLocked||!state?.turns.length} onClick={()=>{setObserved(null);void lab.reset();}}>{t("New chat")}</button>
      </div>
      {configure&&<div className="lab-chat-config">
        <label className="field"><span className="label">{t("Model")}</span><input value={lab.model} disabled={configLocked} onChange={event=>lab.setModel(event.target.value)}/></label>
        <label className="field"><span className="label">{t("Autopilot interval (seconds)")}</span><input type="number" min={5} max={120} value={interval} disabled={configLocked} onChange={event=>setInterval(Number(event.target.value))}/></label>
        <label><input type="checkbox" checked={lab.schemaGuided} disabled={configLocked} onChange={event=>lab.setSchemaGuided(event.target.checked)}/>{t("Constrain output schema")}</label>
        <details className="lab-score"><summary>{t("Experimental prompt")}</summary><textarea aria-label={t("Experimental prompt")} rows={9} maxLength={16000} spellCheck={false} value={lab.prompt} disabled={configLocked} onChange={event=>lab.setPrompt(event.target.value)}/></details>
      </div>}
      <div className="lab-session-bar">
        <label><input type="checkbox" checked={testing?session?.live:live} disabled={configLocked} onChange={event=>setLive(event.target.checked)}/>{t("Live motion")}</label>
        <label><input type="checkbox" checked={testing?session?.autopilot:autopilot} disabled={configLocked} onChange={event=>setAutopilot(event.target.checked)}/>{t("Autopilot")}</label>
        {testing?<span role="status" className="hint">{session?.live?t("Live test running"):t("Preview test running")}</span>:<button className="btn btn-start" disabled={lab.locked||(!live&&!autopilot)||(live&&motion?.available===false)} onClick={()=>void lab.startSession(live,autopilot,interval)}>{t("Start test")}</button>}
        <button className="btn btn-secondary" onClick={()=>void stop()}>{t("Stop")}</button><LabHelpLink section="autopilot"/>
        {app?.motion_simulated&&<span role="status" className="hint">{t("Simulation active · device will not move")}</span>}
        {session?.error&&<span role="alert" className="form-status">{translateKnown(session.error)}</span>}
      </div>
      <div className="lab-chat-log" ref={log} role="log" aria-label={t("Lab messages")} aria-live="polite" aria-relevant="additions text">
        {!state&&!lab.error&&<p role="status" className="hint">{t("Loading…")}</p>}
        {state?.turns.length===0&&!lab.pendingMessage&&<div className="lab-chat-welcome"><h2>{t("Describe the motion you want to test")}</h2><p>{t("Chat naturally. Each reply uses the selected test mode.")}</p>
          <div className="lab-suggestions">{[t("Hold the tip while varying the reach."),t("Make the range change more gradually."),t("Explain the current motion without changing it.")].map(example=><button key={example} disabled={lab.locked} onClick={()=>{setMessage(example);composer.current?.focus();}}>{example}</button>)}</div>
        </div>}
        {state?.turns.map((turn,index)=><div className="lab-exchange" key={`${state.revision-state.turns.length+index}:${turn.message}`}>
          {!turn.autopilot&&<div className="chat-message user" data-role="user"><div className="chat-body"><div className="chat-speaker">{t("You")}</div><div className="chat-bubble">{turn.message}</div></div></div>}
          <div className="chat-message assistant" data-role="assistant"><div className="chat-body">
            <div className="chat-speaker">{turn.autopilot?t("Autopilot"):t("Assistant")} <span className="hint">{turn.motion_applied?t("Motion updated"):turn.valid?(turn.changed.length?t("Preview updated"):t("No changes")):t("Rejected")}</span></div>
            <div className="chat-bubble">{turn.reply||turn.error}</div>
            {turn.error&&turn.reply&&<p className="form-status">{turn.error}</p>}{turn.motion_error&&<p className="form-status" role="alert">{turn.motion_error}</p>}
            <div className="lab-reply-actions"><button className="lab-text-button" disabled={lab.locked} onClick={()=>setObserved({index,label:turn.message,target:{source:"llm",settings_key:state.settings_key,revision:state.revision,turn_index:index}})}>{t("Observe reply")}</button><LabHelpLink section="storage"/>
              <details className="lab-score"><summary>{t("Response details")}</summary><p className="hint">{t("{model} · {time} ms · {calls} provider calls",{model:turn.model,time:turn.elapsed_ms,calls:turn.provider_calls})}</p>
                <p>{turn.recipe_name||turn.changed.join(", ")||t("No changes")}</p><pre>{turn.raw}</pre><CreateTestSequence disabled={lab.locked} target={{source:"llm",settings_key:state.settings_key,revision:state.revision,turn_index:index}}/>
              </details>
            </div>
            {observed?.index===index&&<ObservationEditor key={`${observed.target.revision}:${index}`} target={observed.target} label={observed.label} close={()=>setObserved(null)}/>}
          </div></div>
        </div>)}
        {lab.pendingMessage&&<div className="lab-exchange"><div className="chat-message user" data-role="user"><div className="chat-body"><div className="chat-speaker">{t("You")}</div><div className="chat-bubble">{lab.pendingMessage}</div></div></div><p role="status" className="hint">{t("Generating…")}</p></div>}
        {state?.busy&&!lab.busy&&<p role="status" className="hint">{session?.autopilot?t("Autopilot is thinking…"):t("Another client is generating a lab reply.")}</p>}
      </div>
      <form className="lab-composer" onSubmit={event=>{event.preventDefault();void send();}}>
        {lab.error&&<p role="alert" className="form-status">{translateKnown(lab.error)} {!state&&<button type="button" className="lab-text-button" onClick={lab.retry}>{t("Retry")}</button>}</p>}
        <div className="lab-compose-row"><textarea ref={composer} aria-label={t("Message")} placeholder={t("Describe a change or ask a question…")} rows={2} maxLength={2000} value={message} disabled={lab.locked} onChange={event=>setMessage(event.target.value)} onKeyDown={event=>{if(event.key==="Enter"&&!event.shiftKey&&!event.nativeEvent.isComposing&&event.keyCode!==229){event.preventDefault();void send();}}}/>
          {lab.pendingMessage?<button type="button" className="btn btn-secondary" onClick={lab.cancel}>{t("Cancel generation")}</button>:<button type="submit" className="btn btn-primary" disabled={lab.locked||!message.trim()||!lab.prompt.trim()}>{t("Send")}</button>}
        </div>
        <div className="lab-chat-footer"><span className="hint">{t("Enter to send · Shift+Enter for a new line")}</span><LabHelpLink section="conversation"/><button type="button" className="lab-text-button" disabled={!state?.turns.length} onClick={()=>exportLabReport("llm-lab-trials.json",{...state,motion_simulated:app?.motion_simulated})}>{t("Export conversation")}</button></div>
      </form>
    </section>
    <details className="lab-panel lab-output"><summary>{t("Motion output")}</summary>{preview&&candidate&&<FlowComparison preview={preview} selected={candidate} compact/>}<details className="lab-score"><summary>{t("Current score")}</summary><pre>{JSON.stringify(state?.current,null,2)}</pre></details></details>
  </div>;
}
