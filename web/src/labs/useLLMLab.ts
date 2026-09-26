import {useEffect,useRef,useState} from "react";
import {api} from "../api/client";
import {t} from "../i18n";
import {useAppState} from "../state/app-state";
import {labApi,type FlowPreview,type LabCompareResult,type LLMLabState} from "./api";

// The modes a comparison runs, in display order.
export const mainLabModes=["creative_v2","layered","stroke_ends","groove","plain_words"];

// These modes edit their own score format. Switching into or out of one loads
// its authoritative starting score instead of projecting one in the browser.
const ownScore=(method:string)=>method==="creative_v2"||method==="stroke_ends"||method==="groove"||method==="plain_words";

export interface ModeComparison {message:string;running:boolean;results:Array<{method:string;result?:LabCompareResult;error?:string}>}

const failure=(reason:unknown)=>reason instanceof Error?reason.message:t("Request failed");

export function useLLMLab() {
  const {state:app,backendOnline,readOnly}=useAppState();
  const [state,setState]=useState<LLMLabState|null>(null);
  const [method,setMethod]=useState("layered");
  const [prompt,setPrompt]=useState("");
  const [model,setModel]=useState("");
  const [schemaGuided,setSchemaGuided]=useState(false);
  const [interval,setInterval]=useState(20);
  const [busy,setBusy]=useState(false);
  const [pendingMessage,setPendingMessage]=useState("");
  const [preview,setPreview]=useState<FlowPreview|null>(null);
  const [comparison,setComparison]=useState<ModeComparison|null>(null);
  const [error,setError]=useState("");
  const [reload,setReload]=useState(0);
  const active=useRef<AbortController|null>(null);
  const mounted=useRef(true);
  const stateRef=useRef(state);stateRef.current=state;
  const scoreKey=JSON.stringify(state?.current);
  const savedLimits=JSON.stringify(app?.settings?.motion);
  useEffect(()=>{
    let live=true;mounted.current=true;
    void labApi.state().then(next=>{if(live){
      const last=next.turns[next.turns.length-1];
      const config=next.session?.active?next.session:last;
      setState(next);setModel(config?.model||next.model);setMethod(config?.method||"layered");
      setPrompt(config?.prompt||next.prompts.layered);setSchemaGuided(config?.schema_guided??true);setError("");
    }}).catch(reason=>{if(live)setError(String(reason));});
    const stop=()=>active.current?.abort();
    window.addEventListener("magichandy:emergency-stop",stop);
    return()=>{live=false;mounted.current=false;active.current?.abort();window.removeEventListener("magichandy:emergency-stop",stop);};
  },[reload]);
  useEffect(()=>{
    if(busy)return;
    let live=true;
    let polling=false;
    const timer=window.setInterval(()=>{
      if(polling)return;polling=true;
      void labApi.status().then(status=>{
        const current=stateRef.current;
        if(current&&current.revision===status.revision&&current.busy===status.busy&&JSON.stringify(current.session)===JSON.stringify(status.session))return;
        return labApi.state().then(next=>{if(live){
          setState(next);
          if(next.session?.active){setMethod(next.session.method);setPrompt(next.session.prompt);setModel(next.session.model||next.model);setSchemaGuided(next.session.schema_guided);}
        }});
      }).catch(()=>{}).finally(()=>{polling=false;});
    },1500);
    return()=>{live=false;window.clearInterval(timer);};
  },[busy]);
  useEffect(()=>{
    if(!state||!backendOnline)return;
    const controller=new AbortController();setPreview(null);
    void labApi.preview(state.current,controller.signal).then(result=>{if(!controller.signal.aborted)setPreview(result);})
      .catch(reason=>{if(!controller.signal.aborted)setError(String(reason));});
    return()=>controller.abort();
  },[scoreKey,savedLimits,backendOnline]);
  const locked=readOnly||!backendOnline||!state||busy||Boolean(state?.busy&&!state?.session?.autopilot);
  async function send(message:string):Promise<boolean> {
    if(!state||locked||!message.trim()||!prompt.trim()||active.current)return false;
    const controller=new AbortController();active.current=controller;setBusy(true);setPendingMessage(message);setError("");
    try {
      const next=await labApi.chat({message,method,prompt,model,revision:state.revision,schema_guided:schemaGuided},controller.signal);
      if(!controller.signal.aborted&&mounted.current){setState(next);return true;}
    } catch(reason) {
      if(mounted.current)setError(controller.signal.aborted?t("Generation canceled. The draft was kept."):failure(reason));
      void labApi.state().then(next=>{if(mounted.current)setState(next);}).catch(()=>{});
    } finally {
      if(mounted.current){setBusy(false);setPendingMessage("");}
      active.current=null;
    }
    return false;
  }
  // Runs one change under the busy flag, keeping any error on screen.
  async function guarded(work:()=>Promise<void>) {
    if(locked)return;setBusy(true);setError("");
    try {await work();}
    catch(reason) {if(mounted.current)setError(failure(reason));}
    finally {if(mounted.current)setBusy(false);}
  }
  const reset=()=>guarded(async()=>{const next=await labApi.reset(undefined,method);if(mounted.current)setState(next);});
  // Live motion and Autopilot take effect at once. A running test is stopped
  // first; turning both off leaves it stopped.
  const setSession=(live:boolean,autopilot:boolean)=>guarded(async()=>{
    if(state?.session?.active)await api.stopMotion();
    const next=live||autopilot?await labApi.session({live,autopilot,interval_seconds:interval,method,prompt,model,schema_guided:schemaGuided}):await labApi.state();
    if(mounted.current)setState(next);
  });
  // Changing mode during a test restarts it in the new mode, from that mode's
  // starting score when it needs its own.
  const chooseMethod=(value:string)=>value===method?Promise.resolve():guarded(async()=>{
    const running=state?.session?.active?state.session:null;
    const nextPrompt=state?.prompts[value]??"";
    const nextSchema=value!=="controls";
    if(running)await api.stopMotion();
    let next=state;
    if(ownScore(value)||ownScore(method)||state?.current.gesture||state?.current.strokes)next=await labApi.reset(undefined,value);
    else if(running)next=await labApi.state();
    if(running)next=await labApi.session({live:running.live,autopilot:running.autopilot,interval_seconds:running.interval_seconds,method:value,prompt:nextPrompt,model,schema_guided:nextSchema});
    if(!mounted.current)return;
    if(next)setState(next);
    setMethod(value);setPrompt(nextPrompt);setSchemaGuided(nextSchema);
  });
  // Puts one request to each main mode in turn. Results never touch the Lab
  // conversation, its score or the device.
  async function compare(message:string) {
    if(locked||state?.session?.active||!message.trim()||active.current)return;
    const controller=new AbortController();active.current=controller;setBusy(true);setError("");
    const results:ModeComparison["results"]=mainLabModes.map(mode=>({method:mode}));
    setComparison({message,running:true,results:[...results]});
    try {
      for(const [index,mode] of mainLabModes.entries()) {
        try {results[index]={method:mode,result:await labApi.compare({message,method:mode,model,schema_guided:true},controller.signal)};}
        catch(reason) {if(controller.signal.aborted)break;results[index]={method:mode,error:failure(reason)};}
        if(mounted.current)setComparison({message,running:true,results:[...results]});
      }
    } finally {
      if(mounted.current){setComparison({message,running:false,results:[...results]});setBusy(false);}
      active.current=null;
    }
  }
  return {state,method,prompt,model,schemaGuided,interval,busy,pendingMessage,preview,comparison,error,locked,
    fresh:!!preview&&JSON.stringify(preview.spec)===scoreKey&&JSON.stringify(preview.settings)===savedLimits,
    setPrompt,setModel,setSchemaGuided,setInterval,chooseMethod,send,reset,setSession,compare,
    closeComparison:()=>setComparison(null),cancel:()=>active.current?.abort(),retry:()=>setReload(value=>value+1)};
}
