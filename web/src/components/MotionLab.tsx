import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";
import { exportLabReport, initialFlow, labApi, type FlowPreview, type FlowSpec, type ObservationTarget } from "../labs/api";
import { FlowComparison, auditionLabel } from "../labs/FlowComparison";
import { ObservationEditor } from "../labs/Observations";
import { LabHelpLink } from "../labs/LabHelp";
import { CreateTestSequence } from "../labs/CreateTestSequence";
import "../styles/motion-lab.css";

export function MotionLab() {
  const {state, motion, backendOnline, readOnly, refresh} = useAppState();
  const {show} = useToast();
  const [draft,setDraft] = useState<FlowSpec>(()=>({...initialFlow, speed_percent:Math.max(state?.settings?.motion?.speed_min_percent??1,Math.min(25,state?.settings?.motion?.speed_max_percent??100))}));
  const [preview,setPreview] = useState<FlowPreview|null>(null);
  const [method,setMethod] = useState("flow");
  const [pending,setPending] = useState(true);
  const [starting,setStarting] = useState(false);
  const [error,setError] = useState("");
  const [observed,setObserved] = useState<{target:ObservationTarget;label:string}|null>(null);
  const [scoreText,setScoreText] = useState("");
  const [approved,setApproved] = useState("");
  const generation = useRef(0);
  const draftKey = JSON.stringify(draft);
  const limitsKey = JSON.stringify(state?.settings?.motion);
  const requestKey = draftKey+limitsKey;

  useEffect(()=>{
    const revision=++generation.current;
    const controller=new AbortController();
    setPending(true);setError("");
    if (!backendOnline) return;
    const timer=window.setTimeout(()=>{
      void labApi.preview(draft,controller.signal).then(result=>{
        if(generation.current!==revision) return;
        setPreview(result);setApproved(requestKey);
        setMethod(current=>result.candidates.some(candidate=>candidate.method===current)?current:"flow");
      }).catch((reason:unknown)=>{
        if(!controller.signal.aborted && generation.current===revision) setError(reason instanceof Error?reason.message:t("Request failed"));
      }).finally(()=>{if(generation.current===revision)setPending(false);});
    },180);
    return ()=>{controller.abort();window.clearTimeout(timer);};
  },[backendOnline,requestKey]);

  const selected=preview?.candidates.find(candidate=>candidate.method===method);
  const fresh=!!preview && approved===requestKey && !pending && !error;
  function patch<K extends keyof FlowSpec>(key:K,value:FlowSpec[K]) {
    setDraft(current=>{
      const next={...current,[key]:value};
      next.range_floor_percent=Math.min(next.range_floor_percent,next.max_percent-next.min_percent);
      return next;
    });
  }
  function example(kind:"flow"|"layers"|"sequence") {
    setDraft(current=>{
      const next={...current};delete next.steps;delete next.layers;
      if(kind==="layers")next.layers=[{axis:"range",amount_percent:40,period_cycles:8,phase_percent:0},{axis:"center",amount_percent:25,period_cycles:16,phase_percent:25},{axis:"pace",amount_percent:20,period_cycles:12,phase_percent:0}];
      if(kind==="sequence")next.steps=[{min_percent:current.min_percent,max_percent:current.max_percent,speed_percent:current.speed_percent,cycles:4},
        {min_percent:current.min_percent,max_percent:current.min_percent+Math.max(10,Math.round((current.max_percent-current.min_percent)*.6)),speed_percent:current.speed_percent,cycles:3}];
      return next;
    });
  }
  async function start() {
    if(!preview||!selected||!fresh||starting)return;
    setStarting(true);
    try{await labApi.start(preview,selected);}catch(reason){show(reason instanceof Error?reason.message:t("Request failed"),"error");}
    finally{setStarting(false);refresh();}
  }
  async function applyJSON() {
    try{const checked=await labApi.preview(JSON.parse(scoreText) as FlowSpec);setDraft(checked.spec);setScoreText("");}
    catch(reason){show(reason instanceof Error?reason.message:t("Request failed"),"error");}
  }
  const sequenced=Boolean(draft.steps?.length);
  const continuous=method==="flow";
  const minimumSpan=continuous?10:20;
  return <div className="motion-lab">

    {state?.motion_simulated&&<p className="hint-block" role="status">{t("Simulation is active. Motion commands go to the simulator; connected devices will not move.")}</p>}
    <div className="motion-lab-grid">
      <div className="lab-panel motion-lab-controls">
        <div className="lab-panel-heading"><h2>{t("Motion controls")}</h2><LabHelpLink section="motion"/></div>
        <label className="field"><span className="label">{t("Test generator")}</span><select value={method} onChange={event=>setMethod(event.target.value)}>
          <option value="flow">{t("Continuous flow")}</option>
          <option value="creative" disabled={!preview?.candidates.some(candidate=>candidate.method==="creative")}>{t("Creative baseline")}</option>
          <option value="anchored" disabled={!preview?.candidates.some(candidate=>candidate.method==="anchored")}>{t("Anchored range")}</option>
        </select></label>

        {continuous?<fieldset><legend>{t("Score examples")}</legend><div className="row-actions">
          <button className="btn btn-secondary" onClick={()=>example("flow")}>{t("Single section")}</button>
          <button className="btn btn-secondary" onClick={()=>example("layers")}>{t("Layered score")}</button>
          <button className="btn btn-secondary" onClick={()=>example("sequence")}>{t("Section sequence")}</button>
        </div></fieldset>
          :<p className="hint">{t("Historical settings: wander profile, 30% variation, minimum span 20.")}</p>}
        <fieldset><legend>{t("Pace and range")}</legend>
        <LabSlider label={t("Speed")} value={draft.speed_percent} min={state?.settings?.motion?.speed_min_percent??1} max={state?.settings?.motion?.speed_max_percent??100} disabled={continuous&&sequenced} change={value=>patch("speed_percent",value)}/>
        <LabSlider label={t("Outer minimum")} value={draft.min_percent} min={0} max={draft.max_percent-minimumSpan} disabled={continuous&&sequenced} change={value=>patch("min_percent",value)}/>
        <LabSlider label={t("Outer maximum")} value={draft.max_percent} min={draft.min_percent+minimumSpan} max={100} disabled={continuous&&sequenced} change={value=>patch("max_percent",value)}/>
        <LabSlider label={t("Shortest span")} value={Math.max(minimumSpan,draft.range_floor_percent)} min={minimumSpan} max={draft.max_percent-draft.min_percent} change={value=>patch("range_floor_percent",value)}/>
        <LabSlider label={t("Range anchor")} value={method==="creative"?50:draft.anchor_percent} min={0} max={100} disabled={method==="creative"} change={value=>patch("anchor_percent",value)}/>
        <LabHelpLink section="motion"/>
        {continuous&&sequenced&&<p className="hint">{t("Section ranges and speeds are defined in Score JSON.")}</p>}
        </fieldset>{continuous&&<fieldset><legend>{t("Gradual variation")}</legend>
        <label className="field"><span className="label">{t("Variation source")}</span><select value={draft.variation_mode||"waves"} onChange={event=>patch("variation_mode",event.target.value as "waves"|"drift")}><option value="waves">{t("Smooth waves")}</option><option value="drift">{t("Correlated drift")}</option></select></label>
        <LabSlider label={t("Range memory (cycles)")} value={draft.memory_cycles} min={2} max={32} change={value=>patch("memory_cycles",value)}/>
        <LabSlider label={t("Pace variation")} value={draft.pace_variation_percent} min={0} max={40} change={value=>patch("pace_variation_percent",value)}/>

        </fieldset>}
        {continuous&&<fieldset><legend>{t("Motion experiments")}</legend>
          <LabSlider label={t("Turn softness")} value={draft.turn_softness_percent??0} min={0} max={100} change={value=>patch("turn_softness_percent",value)}/>

          <LabSlider label={t("Steady beat")} value={draft.cadence_hold_percent??0} min={0} max={100} change={value=>patch("cadence_hold_percent",value)}/>

        </fieldset>}
        <label className="field"><span className="label">{t("Repeatable seed")}</span><input type="number" min={1} max={2147483647} value={draft.seed} onChange={event=>patch("seed",Math.max(1,Math.min(2147483647,Math.trunc(Number(event.target.value))||1)))}/></label>
        {continuous&&<details className="lab-score"><summary>{t("Score JSON")}</summary>

          <textarea aria-label={t("Score JSON")} rows={14} spellCheck={false} value={scoreText||JSON.stringify(draft,null,2)} onChange={event=>setScoreText(event.target.value)}/>
          <button className="btn btn-secondary" disabled={!scoreText} onClick={()=>void applyJSON()}>{t("Apply score")}</button>
        </details>}
      </div>
      <div className="lab-panel motion-lab-results" aria-busy={pending}>
        {error&&<p className="form-status" role="alert">{translateKnown(error)}</p>}
        {preview&&selected&&<>
          <FlowComparison preview={preview} selected={selected}/>
          <p className="lab-test-target" role="status">{t("Test target: {target}",{target:auditionLabel(method,preview.spec)})}</p>
          <div className="row-actions">
            <button className="btn btn-start" disabled={!fresh||readOnly||!backendOnline||starting||motion?.available===false} onClick={()=>void start()}>{starting?t("Starting…"):state?.motion_simulated?t("Start simulated test"):t("Start selected test")}</button>
            <button className="btn btn-secondary" onClick={()=>void api.stopMotion().catch(reason=>show(String(reason),"error")).finally(refresh)}>{t("Stop")}</button>
            <button className="btn btn-secondary" disabled={!fresh} onClick={()=>exportLabReport("motion-lab-flow.json",{preview,selected:method,motion_simulated:state?.motion_simulated,exported_at:new Date().toISOString()})}>{t("Export comparison")}</button>
          </div>

          <div><button className="btn btn-secondary" disabled={!fresh||readOnly||!backendOnline} onClick={()=>setObserved({target:{source:"motion",method,spec:preview.spec,settings_key:preview.settings_key},label:`${auditionLabel(method,preview.spec)} · ${preview.spec.min_percent}–${preview.spec.max_percent} · ${preview.spec.speed_percent}%`})}>{t("Observe preview")}</button></div>
          {observed&&<ObservationEditor key={JSON.stringify(observed.target)} target={observed.target} label={observed.label} close={()=>setObserved(null)}/>}
          <div><CreateTestSequence disabled={!fresh} target={{source:"motion",method,spec:preview.spec,settings_key:preview.settings_key}}/></div>
          {continuous&&<div><CreateTestSequence experiments disabled={!fresh} target={{source:"motion",method,spec:preview.spec,settings_key:preview.settings_key}}/></div>}
        </>}
      </div>
    </div>
  </div>;
}

function LabSlider({label,value,min,max,change,disabled=false}:{label:string;value:number;min:number;max:number;change:(value:number)=>void;disabled?:boolean}) {
  const id=useId();
  return <label className="field" htmlFor={id}><span className="label">{label}<output htmlFor={id}>{value}</output></span>
    <input id={id} type="range" min={min} max={max} value={value} disabled={disabled} onChange={event=>change(Number(event.target.value))}/></label>;
}
