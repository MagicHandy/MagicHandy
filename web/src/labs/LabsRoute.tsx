import {useEffect,useState} from "react";
import {WorkspaceHead} from "../components/WorkspaceHead";
import {MotionLab} from "../components/MotionLab";
import {useHashRoute} from "../state/app-state";
import {t} from "../i18n";
import {LLMLab} from "./LLMLab";
import {ObservationsPage} from "./Observations";
import {TestRuns} from "./TestRuns";
import "../styles/motion-lab.css";

export function LabsRoute() {
  const requested=useHashRoute().split("/")[2];
  const section=requested==="motion"||requested==="observations"||requested==="tests"?requested:"chat";
  const [chatDraft,setChatDraft]=useState("");
  const [visited,setVisited]=useState([section]);
  useEffect(()=>setVisited(current=>current.includes(section)?current:[...current,section]),[section]);
  function useInChat(text:string) {setChatDraft(text);window.location.hash="#/labs/chat";}
  return <div className="labs-route">
    <div className="labs-heading"><WorkspaceHead title={t("Labs")}/><span className="hint">{t("Experimental workspace")}</span></div>
    <nav className="labs-tabs" aria-label={t("Lab workspaces")}>
      {([["chat",t("LLM Lab")],["motion",t("Motion Lab")],["tests",t("Guided tests")],["observations",t("Observations")]] as const).map(([id,label])=><a key={id} href={`#/labs/${id}`} aria-current={section===id?"page":undefined}>{label}</a>)}
    </nav>
    {(section==="chat"||visited.includes("chat"))&&<div className="lab-tab-panel" hidden={section!=="chat"}><LLMLab initialDraft={chatDraft} draftUsed={()=>setChatDraft("")}/></div>}
    {(section==="motion"||visited.includes("motion"))&&<div className="lab-tab-panel" hidden={section!=="motion"}><MotionLab/></div>}
    {section==="observations"&&<ObservationsPage useInChat={useInChat}/>}
    {section==="tests"&&<TestRuns/>}
  </div>;
}
