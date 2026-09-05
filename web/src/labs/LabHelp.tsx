import {useEffect,useRef,useState} from "react";
import {t} from "../i18n";
import {useHashRoute} from "../state/app-state";
import {labApi} from "./api";

export function LabHelpLink({section}:{section:string}) {
  return <a className="lab-help-link" href={`#/labs/help/${section}`}>{t("Help")}</a>;
}

export function LabHelp() {
  const requested=useHashRoute().split("/")[3]||"conversation";
  const topics=["conversation","modes","autopilot","motion","feedback","storage"];
  const topic=topics.includes(requested)?requested:"conversation";
  const labels=[t("Live conversation"),t("Test modes"),t("Autopilot"),t("Motion controls"),t("Guided tests"),t("Storage and use")];
  const [path,setPath]=useState("");
  const heading=useRef<HTMLHeadingElement>(null);
  useEffect(()=>{let active=true;void labApi.observations().then(data=>{if(active)setPath(data.storage_path);}).catch(()=>{});return()=>{active=false;};},[]);
  useEffect(()=>{heading.current?.focus();},[topic]);
  return <section className="lab-help-page">
    <nav className="lab-help-topics" aria-label={t("Help topics")}>{topics.map((id,index)=><a key={id} href={`#/labs/help/${id}`} aria-current={id===topic?"page":undefined}>{labels[index]}</a>)}</nav>
    <article className="lab-help-content"><h2 ref={heading} tabIndex={-1}>{labels[topics.indexOf(topic)]}</h2>
      {topic==="conversation"&&<>
        <p>{t("Choose a test mode and chat normally. Enable Live motion and press Start test to apply accepted replies to the device. Without Live motion, replies update only the plotted score.")}</p>
        <p>{t("Start replaces current motion and repeats the selected test until Stop. Saved motion limits apply.")}</p>
        <p>{t("One model call per reply; rejected output stays visible. No repair or automatic fallback.")}</p>
        <p>{t("Configure opens model, schema and prompt options. End the test before changing its configuration. Lab prompts and conversation are separate from production chat.")}</p>
        <p>{t("The latest 20 turns stay in this app session. New chat or an app restart clears them. Saved observations remain. Export the conversation to keep all available replies and prompts.")}</p>
      </>}
      {topic==="modes"&&<>
        <p>{t("Changing the interface loads its default prompt. The next reply uses the selected model, prompt and matching conversation history.")}</p>
        <dl><dt>{t("Creative v2")}</dt><dd>{t("Generate asymmetric sweeps, localized strokes and shrinking rebounds. Edit focus, sweep, rebounds or inertia while preserving other controls. Switching to or from this generator starts a new Lab score.")}</dd>
          <dt>{t("Layered")}</dt><dd>{t("The production Layered contract: edit reach, location and pace independently. Partial edits preserve other layers. Drift varies irregularly; alternation reaches both extremes. Evolve refreshes the details without replacing the geometry.")}</dd>
          <dt>{t("Relative and layer edits")}</dt><dd>{t("Ask for relative changes or edit one layer. Existing layers and section differences are preserved unless you explicitly change them.")}</dd>
          <dt>{t("Single controls")}</dt><dd>{t("Test direct changes to range, anchor, pace and variation.")}</dd>
          <dt>{t("Sequence")}</dt><dd>{t("Test an ordered list of sections. New sections replace the previous sequence.")}</dd>
          <dt>{t("Layers")}</dt><dd>{t("Test simultaneous range, center and pace modulation on one carrier.")}</dd>
          <dt>{t("New library")}</dt><dd>{t("Compare action names, descriptive IDs and opaque handles for the same pattern catalog. The three modes change naming, not motion.")}</dd>
        </dl>
        <p>{t("Legacy built-ins are disabled on update and cannot be re-enabled. Their saved names and weights remain available with their exports.")}</p>
        <p>{t("Creative v2 uses 32 primary cycles with optional local rebounds. Mixed focus returns to full reach after at most six local cycles. Tiny rebound tails are omitted. Inertia shapes the velocity crest; it does not simulate impacts. Safety limits can reduce timing contrast.")}</p>
      </>}
      {topic==="autopilot"&&<>
        <p>{t("Enable Autopilot and press Start test. It continues this Lab conversation using the selected model, prompt and schema after each quiet interval. You can test it with or without Live motion.")}</p>
        <p>{t("Sending a message interrupts an Autopilot reply. Failed output pauses Autopilot for inspection. Stop ends the session and cancels pending replies, even without a connected device.")}</p>
        <p>{t("Lab Autopilot tests the selected conversation contract. Production Autopilot has additional planning policies and is not changed by these settings.")}</p>
        <p>{t("Layered starts with fresh variation and refreshes it during Autopilot. Exact repetition requests take priority. Four recent human requests remain available independently of automatic replies. Each score is finite; Layered Lab continuations add up to half a quiet interval of timing variation.")}</p>
        <p>{t("Creative v2 also refreshes its realization during Autopilot, preserving the requested character. Without continuation the finite score repeats. Exact repetition requests keep the current realization.")}</p>
      </>}
      {topic==="motion"&&<>
        <p>{t("This generator plays the continuous score below.")}</p>
        <p>{t("These load arrangements for Continuous flow; the generator stays the same.")}</p>
        <p>{t("Historical reference: shares pace, outer band, shortest span and seed. Continuous layers, sections and gradual variation do not apply.")}</p>
        <p>{t("0 holds the base end; 50 contracts around the center; 100 holds the tip end.")}</p>
        <p>{t("Drift uses reproducible irregular trends. Both sources repeat at the end of the score.")}</p>
        <p>{t("Higher values linger near both turnarounds, with more travel in the middle of each stroke.")}</p>
        <p>{t("Keep a steadier cycle time when reach changes. This can lower effective pace; use the preview readout to compare.")}</p>
        <p>{t("Section bands and speeds are edited in the score. Layers modulate one carrier; they do not add independent position commands.")}</p>
        <p>{t("First 12 seconds, semantic position: base 0, tip 100. Dashed line shows the baseline.")}</p>
        <p>{t("Experimental smoothness limits can lower effective pace. Estimates are not physical feedback.")}</p>
      </>}
      {topic==="feedback"&&<>
        <p>{t("Five rounds compare a continuous reference, correlated drift, softer reversals, a steadier beat and their combination.")}</p>
        <p>{t("Choose Observe preview in Motion Lab or Observe reply beside an LLM response, then save your observation.")}</p>
        <p>{t("Describe whether you reviewed the plotted estimate or felt a device audition, and include the transport when relevant.")}</p>
        <p>{t("Create a guided sequence from a preview or reply, follow its rounds, and rate each test with optional comments. Saved feedback is evidence for later review; it does not train the model or change motion automatically.")}</p>
      </>}
      {topic==="storage"&&<>
        <p>{t("Sequences, captured previews and saved answers stay in this app’s database across restarts. Unsaved comments stay only on this page.")}</p>
        <p>{t("Feedback is available for review and JSON export. It does not automatically change prompts or motion, or get sent to a model.")}</p>
        <p>{t("Observations stay in this app’s local database across restarts and new lab chats. They are review evidence, not model training or automatic instructions.")}</p>
        {path&&<p>{t("Database")} <code className="lab-storage-path">{path}</code></p>}
        <p>{t("Save keeps this observation and its source in the app’s local database. It does not change prompts, preferences or motion. Use in chat copies it to a draft for you to send.")}</p>
        <p>{t("Use in chat opens an editable message. Only sending that message shares the observation with the selected LLM. Export creates a JSON file with the observation and its captured source.")}</p>
        <p>{t("Older unsaved observation fields were export-only; they cannot be recovered here.")}</p>
        <p>{t("The latest 20 turns stay in this app session. New chat or an app restart clears them. Saved observations remain. Export the conversation to keep all available replies and prompts.")}</p>
      </>}
    </article>
  </section>;
}
