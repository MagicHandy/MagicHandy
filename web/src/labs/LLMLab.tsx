import {useEffect,useRef,useState} from "react";
import {api} from "../api/client";
import {t,translateKnown} from "../i18n";
import {useAppState,useToast} from "../state/app-state";
import {exportLabReport,labApi,type ObservationTarget} from "./api";
import {FlowComparison} from "./FlowComparison";
import {ObservationEditor} from "./Observations";
import {useLLMLab} from "./useLLMLab";
import {CreateTestSequence} from "./CreateTestSequence";

export function LLMLab({initialDraft="",draftUsed=()=>{}}:{initialDraft?:string;draftUsed?:()=>void}) {
  const {state:app,motion,refresh}=useAppState();
  const {show}=useToast();
  const lab=useLLMLab();
  const [message,setMessage]=useState("");
  const [starting,setStarting]=useState(false);
  const [observed,setObserved]=useState<{target:ObservationTarget;label:string;index:number}|null>(null);
  const log=useRef<HTMLDivElement>(null);
  const composer=useRef<HTMLTextAreaElement>(null);
  const {state,preview}=lab;
  useEffect(()=>{if(initialDraft){setMessage(initialDraft);draftUsed();composer.current?.focus();}},[initialDraft,draftUsed]);
  useEffect(()=>{if(log.current)log.current.scrollTop=log.current.scrollHeight;},[state?.revision,lab.pendingMessage]);
  async function send() {if(await lab.send(message.trim())){setMessage("");composer.current?.focus();}}
  async function audition() {
    if(!preview||!lab.fresh||lab.locked||starting)return;
    const candidate=preview.candidates.find(item=>item.flow);if(!candidate)return;
    setStarting(true);
    try {await labApi.start(preview,candidate);}
    catch(reason) {show(reason instanceof Error?reason.message:t("Request failed"),"error");}
    finally {setStarting(false);refresh();}
  }
  const candidate=preview?.candidates.find(item=>item.flow);
  return <div className="llm-lab-workspace">
    <section className="lab-chat" aria-label={t("LLM Lab conversation")}>
      <div className="lab-chat-toolbar">
        <div><strong>{t("LLM Lab")}</strong><span className="hint">{t("Replies update the preview. Audition starts motion.")}</span></div>
        <button className="btn btn-secondary" disabled={lab.locked||!state?.turns.length} onClick={()=>{setObserved(null);void lab.reset();}}>{t("New chat")}</button>
      </div>
      <div className="lab-chat-log" ref={log} role="log" aria-label={t("Lab messages")} aria-live="polite" aria-relevant="additions text">
        {!state&&!lab.error&&<p role="status" className="hint">{t("Loading…")}</p>}
        {state?.turns.length===0&&!lab.pendingMessage&&<div className="lab-chat-welcome">
          <span className="lab-eyebrow">{t("A separate conversation")}</span><h2>{t("Describe the motion you want to test")}</h2>
          <p>{t("Ask for a change, inspect the response and preview, then audition when ready.")}</p>
          <div className="lab-suggestions">{[t("Hold the tip while varying the reach."),t("Make the range change more gradually."),t("Explain the current motion without changing it.")].map(example=><button key={example} disabled={lab.locked} onClick={()=>{setMessage(example);composer.current?.focus();}}>{example}</button>)}</div>
        </div>}
        {state?.turns.map((turn,index)=><div className="lab-exchange" key={`${state.revision-state.turns.length+index}:${turn.message}`}>
          <div className="chat-message user" data-role="user"><div className="chat-body"><div className="chat-speaker">{t("You")}</div><div className="chat-bubble">{turn.message}</div></div></div>
          <div className="chat-message assistant" data-role="assistant"><div className="chat-body">
            <div className="chat-speaker">{t("Assistant")} <span className="hint">{turn.valid?(turn.changed.length?t("Preview updated"):t("No changes")):t("Rejected")}</span></div>
            <div className="chat-bubble">{turn.reply||turn.error}</div>
            {turn.error&&turn.reply&&<p className="form-status">{turn.error}</p>}
            <div className="lab-reply-actions"><details className="lab-score"><summary>{t("Response details")}</summary>
              <p className="hint">{t("{model} · {time} ms · {calls} provider calls",{model:turn.model,time:turn.elapsed_ms,calls:turn.provider_calls})}</p>
              {turn.recipe_name&&<p>{turn.recipe_name}</p>}<p className="hint">{turn.changed.join(", ")||t("No changes")}</p><pre>{turn.raw}</pre>
            </details><button className="lab-text-button" disabled={lab.locked} onClick={()=>setObserved({index,label:turn.message,target:{source:"llm",settings_key:state.settings_key,revision:state.revision,turn_index:index}})}>{t("Observe reply")}</button></div>
            <CreateTestSequence disabled={lab.locked} target={{source:"llm",settings_key:state.settings_key,revision:state.revision,turn_index:index}}/>
            {observed?.index===index&&<ObservationEditor key={`${observed.target.revision}:${index}`} target={observed.target} label={observed.label} close={()=>setObserved(null)}/>}
          </div></div>
        </div>)}
        {lab.pendingMessage&&<div className="lab-exchange"><div className="chat-message user" data-role="user"><div className="chat-body"><div className="chat-speaker">{t("You")}</div><div className="chat-bubble">{lab.pendingMessage}</div></div></div><p role="status" className="hint">{t("Generating…")}</p></div>}
        {state?.busy&&!lab.busy&&<p role="status" className="hint">{t("Another client is generating a lab reply.")}</p>}
      </div>
      <form className="lab-composer" onSubmit={event=>{event.preventDefault();void send();}}>
        {lab.error&&<p role="alert" className="form-status">{translateKnown(lab.error)} {!state&&<button type="button" className="lab-text-button" onClick={lab.retry}>{t("Retry")}</button>}</p>}
        <div className="lab-compose-row"><textarea ref={composer} aria-label={t("Message")} placeholder={t("Describe a change or ask a question…")} rows={2} maxLength={2000} value={message} disabled={lab.locked}
          onChange={event=>setMessage(event.target.value)} onKeyDown={event=>{if(event.key==="Enter"&&!event.shiftKey&&!event.nativeEvent.isComposing&&event.keyCode!==229){event.preventDefault();void send();}}}/>
          {lab.pendingMessage?<button type="button" className="btn btn-secondary" onClick={lab.cancel}>{t("Cancel generation")}</button>:<button type="submit" className="btn btn-primary" disabled={lab.locked||!message.trim()||!lab.prompt.trim()}>{t("Send")}</button>}
        </div><p className="hint">{t("Enter to send · Shift+Enter for a new line")}</p>
      </form>
    </section>
    <aside className="lab-inspector" aria-label={t("Lab tools")}>
      <section className="lab-panel"><h2>{t("Current preview")}</h2>
        {app?.motion_simulated&&<p className="hint-block" role="status">{t("Simulation is active. Motion commands go to the simulator; connected devices will not move.")}</p>}
        {preview&&candidate?<FlowComparison preview={preview} selected={candidate} compact/>:<p className="hint">{t("Preparing preview…")}</p>}
        <div className="row-actions"><button className="btn btn-start" disabled={!lab.fresh||lab.locked||starting||motion?.available===false} onClick={()=>void audition()}>{starting?t("Starting…"):t("Audition proposal")}</button>
          <button className="btn btn-secondary" onClick={()=>void api.stopMotion().catch(reason=>show(String(reason),"error")).finally(refresh)}>{t("Stop")}</button></div>
        <p className="hint">{t("Start replaces current motion and repeats the selected test until Stop. Saved motion limits apply.")}</p>
      </section>
      <details className="lab-panel lab-setup"><summary>{t("Experiment setup")}</summary>
        <p className="hint lab-model-summary">{lab.model||t("Loading…")}</p>
        <label className="field"><span className="label">{t("Control interface")}</span><select value={lab.method.startsWith("library")?"library":lab.method} disabled={lab.locked} onChange={event=>lab.chooseMethod(event.target.value)}>
          <option value="controls">{t("Single controls")}</option><option value="sequence">{t("Sequence")}</option><option value="layers">{t("Layers")}</option><option value="library">{t("New library")}</option>
          <option value="edits">{t("Relative and layer edits")}</option>
        </select></label>
        {lab.method==="edits"&&<p className="hint">{t("Ask for relative changes or edit one layer. Existing layers and section differences are preserved unless you explicitly change them.")}</p>}
        {lab.method.startsWith("library")&&<label className="field"><span className="label">{t("Recipe naming")}</span><select value={lab.method} disabled={lab.locked} onChange={event=>lab.chooseMethod(event.target.value)}><option value="library">{t("Opaque handles")}</option><option value="library_descriptive">{t("Descriptive IDs")}</option><option value="library_actions">{t("Action names")}</option></select></label>}
        <label className="field"><span className="label">{t("Model")}</span><input type="text" value={lab.model} disabled={lab.locked} onChange={event=>lab.setModel(event.target.value)}/></label>
        <label className="field"><span><input type="checkbox" checked={lab.schemaGuided} disabled={lab.locked} onChange={event=>lab.setSchemaGuided(event.target.checked)}/>{t("Constrain output schema")}</span></label>
        <details className="lab-score"><summary>{t("Experimental prompt")}</summary><textarea aria-label={t("Experimental prompt")} rows={12} maxLength={16000} spellCheck={false} value={lab.prompt} disabled={lab.locked} onChange={event=>lab.setPrompt(event.target.value)}/></details>
        <p className="hint">{t("Changing the interface loads its default prompt. The next reply uses the selected model, prompt and matching conversation history.")}</p>
      </details>
      <details className="lab-panel lab-score"><summary>{t("Current score")}</summary><pre>{JSON.stringify(state?.current,null,2)}</pre></details>
      <details className="lab-panel"><summary>{t("Conversation storage")}</summary>
        <p className="hint">{t("The latest 20 turns stay in this app session. New chat or an app restart clears them. Saved observations remain. Export the conversation to keep all available replies and prompts.")}</p>
        <p className="hint">{t("One model call per reply; rejected output stays visible. No repair or automatic fallback.")}</p>
        <button className="btn btn-secondary" disabled={!state?.turns.length} onClick={()=>exportLabReport("llm-lab-trials.json",{...state,motion_simulated:app?.motion_simulated})}>{t("Export conversation")}</button>
      </details>
    </aside>
  </div>;
}
