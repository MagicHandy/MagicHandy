import {useEffect,useRef,useState} from "react";
import {t} from "../i18n";
import {useAppState} from "../state/app-state";
import {labApi,type FlowPreview,type LLMLabState} from "./api";

export function useLLMLab() {
  const {state:app,backendOnline,readOnly}=useAppState();
  const [state,setState]=useState<LLMLabState|null>(null);
  const [method,setMethod]=useState("layered");
  const [prompt,setPrompt]=useState("");
  const [model,setModel]=useState("");
  const [schemaGuided,setSchemaGuided]=useState(false);
  const [busy,setBusy]=useState(false);
  const [pendingMessage,setPendingMessage]=useState("");
  const [preview,setPreview]=useState<FlowPreview|null>(null);
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
      if(mounted.current)setError(controller.signal.aborted?t("Generation canceled. The draft was kept."):reason instanceof Error?reason.message:t("Request failed"));
      void labApi.state().then(next=>{if(mounted.current)setState(next);}).catch(()=>{});
    } finally {
      if(mounted.current){setBusy(false);setPendingMessage("");}
      active.current=null;
    }
    return false;
  }
  async function reset() {
    if(!state||locked)return;setBusy(true);setError("");
    try {const next=await labApi.reset(undefined,method);if(mounted.current)setState(next);}
    catch(reason) {if(mounted.current)setError(String(reason));}
    finally {if(mounted.current)setBusy(false);}
  }
  async function chooseMethod(value:string) {
    if(locked||state?.session?.active)return;
    // A different generator needs its own authoritative starting score. Never
    // project a second score in the browser or erase a running test's state.
    if(value==="creative_v2"||state?.current.gesture){
      setBusy(true);setError("");
      try {setState(await labApi.reset(undefined,value));}
      catch(reason){setError(String(reason));return;}
      finally {setBusy(false);}
    }
    setMethod(value);setPrompt(state?.prompts[value]??"");setSchemaGuided(value!=="controls");
  }
  async function startSession(live:boolean,autopilot:boolean,interval:number) {
    if(locked)return;setBusy(true);setError("");
    try {setState(await labApi.session({live,autopilot,interval_seconds:interval,method,prompt,model,schema_guided:schemaGuided}));}
    catch(reason){setError(reason instanceof Error?reason.message:t("Request failed"));}
    finally {setBusy(false);}
  }
  return {state,method,prompt,model,schemaGuided,busy,pendingMessage,preview,error,locked,
    fresh:!!preview&&JSON.stringify(preview.spec)===scoreKey&&JSON.stringify(preview.settings)===savedLimits,
    setPrompt,setModel,setSchemaGuided,chooseMethod,send,reset,startSession,cancel:()=>active.current?.abort(),retry:()=>setReload(value=>value+1)};
}
