import {useEffect,useRef,useState} from "react";
import {t} from "../i18n";
import {useAppState} from "../state/app-state";
import {initialFlow,labApi,type FlowPreview,type LLMLabState} from "./api";

export function useLLMLab() {
  const {state:app,backendOnline,readOnly}=useAppState();
  const [state,setState]=useState<LLMLabState|null>(null);
  const [method,setMethod]=useState("controls");
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
  const scoreKey=JSON.stringify(state?.current);
  const savedLimits=JSON.stringify(app?.settings?.motion);
  useEffect(()=>{
    let live=true;mounted.current=true;
    void labApi.state().then(next=>{if(live){
      const last=next.turns[next.turns.length-1];
      setState(next);setModel(last?.model||next.model);setMethod(last?.method||"controls");
      setPrompt(last?.prompt||next.prompts.controls);setSchemaGuided(last?.schema_guided??false);setError("");
    }}).catch(reason=>{if(live)setError(String(reason));});
    const stop=()=>active.current?.abort();
    window.addEventListener("magichandy:emergency-stop",stop);
    return()=>{live=false;mounted.current=false;active.current?.abort();window.removeEventListener("magichandy:emergency-stop",stop);};
  },[reload]);
  useEffect(()=>{
    if(!state?.busy||busy)return;
    let live=true;
    const timer=window.setInterval(()=>void labApi.state().then(next=>{if(live)setState(next);}).catch(()=>{}),1500);
    return()=>{live=false;window.clearInterval(timer);};
  },[state?.busy,busy]);
  useEffect(()=>{
    if(!state||!backendOnline)return;
    const controller=new AbortController();setPreview(null);
    void labApi.preview(state.current,controller.signal).then(result=>{if(!controller.signal.aborted)setPreview(result);})
      .catch(reason=>{if(!controller.signal.aborted)setError(String(reason));});
    return()=>controller.abort();
  },[scoreKey,savedLimits,backendOnline]);
  const locked=readOnly||!backendOnline||!state||busy||Boolean(state?.busy);
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
    try {const next=await labApi.reset({...initialFlow,speed_percent:Math.max(state.limits.speed_min_percent,Math.min(25,state.limits.speed_max_percent))});if(mounted.current)setState(next);}
    catch(reason) {if(mounted.current)setError(String(reason));}
    finally {if(mounted.current)setBusy(false);}
  }
  function chooseMethod(value:string) {setMethod(value);setPrompt(state?.prompts[value]??"");setSchemaGuided(value!=="controls");}
  return {state,method,prompt,model,schemaGuided,busy,pendingMessage,preview,error,locked,
    fresh:!!preview&&JSON.stringify(preview.spec)===scoreKey&&JSON.stringify(preview.settings)===savedLimits,
    setPrompt,setModel,setSchemaGuided,chooseMethod,send,reset,cancel:()=>active.current?.abort(),retry:()=>setReload(value=>value+1)};
}
