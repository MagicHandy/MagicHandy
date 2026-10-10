import {useState} from "react";
import {PatternPlotAxes} from "../components/PatternCurve";
import {t} from "../i18n";
import {LabHelpLink} from "./LabHelp";
import type {FlowCandidate,FlowPreview,FlowSpec} from "./api";

const number=(value:number)=>value.toLocaleString(undefined,{maximumFractionDigits:1});
export const methodLabel=(method:string)=>method==="creative"?t("Creative baseline"):method==="anchored"?t("Anchored range"):t("Continuous flow");
export function scoreArrangementLabel(spec:FlowSpec):string {
  if(spec.steps?.length)return spec.layers?.length?t("Sections with layers"):t("Section sequence");
  return spec.layers?.length?t("Layered score"):t("Single section");
}
export const auditionLabel=(method:string,spec:FlowSpec)=>method==="flow"?t("{generator} · {arrangement}",{generator:methodLabel(method),arrangement:scoreArrangementLabel(spec)}):methodLabel(method);

// Both charts draw the backend samples edge to edge across the time domain they
// cover and carry their scales as hint-size HTML beside and under the drawing
// (PatternPlotAxes), so no label is scaled with it at any width.
export function FlowComparison({preview,selected,compact=false}:{preview:FlowPreview;selected:FlowCandidate;compact?:boolean}) {
  const [references,setReferences]=useState(false);
  const baseline=preview.candidates.find(candidate=>candidate.method==="creative");
  const durationMs=Math.max(1,...[selected,...(baseline?[baseline]:[])].flatMap(candidate=>candidate.samples.map(sample=>sample.time_ms)));
  const velocityMax=Math.max(1,...selected.samples.map(sample=>Math.abs(sample.velocity_percent_per_second)),...(references&&baseline?baseline.samples.map(sample=>Math.abs(sample.velocity_percent_per_second)):[]));
  const path=(candidate:FlowCandidate,velocity=false)=>candidate.samples.map(sample=>`${sample.time_ms/durationMs*700},${velocity?100-sample.velocity_percent_per_second/velocityMax*100:200-sample.position_percent*2}`).join(" ");
  const velocityValue=(value:number)=><span className="pattern-plot-value">{t("{n} %/s",{n:number(value)})}</span>;
  return <div className="lab-flow-preview">
    {!compact&&<div className="lab-panel-heading"><h2>{t("Motion preview")}</h2><span className="hint">{auditionLabel(selected.method,preview.spec)}</span></div>}
    <PatternPlotAxes durationMs={durationMs}><svg className="motion-lab-chart" viewBox="0 0 700 200" preserveAspectRatio="none" role="img" aria-label={t("Planned position comparison")}>
      <line x1={0} x2={700} y1={100} y2={100}/>
      {references&&baseline&&<polyline className="motion-lab-baseline" points={path(baseline)}/>}
      <polyline className="motion-lab-selected" points={path(selected)}/>
    </svg></PatternPlotAxes>
    <dl className="lab-preview-readouts"><div><dt>{t("Reach")}</dt><dd>{number(selected.perceptual.position_min_percent)}–{number(selected.perceptual.position_max_percent)}</dd></div><div><dt>{t("Effective pace")}</dt><dd>{number(selected.perceptual.pace.effective_percent)}%</dd></div></dl>
    <p className="hint">{t("Planned position, base 0 to tip 100. Estimates are not device feedback.")}</p>
    <details className="lab-comparison-details" onToggle={event=>setReferences(event.currentTarget.open)}><summary>{t("Compare methods and dynamics")}</summary><LabHelpLink section="motion"/>

      <div className="lab-table-scroll"><table className="motion-lab-table"><caption>{t("Full-phrase estimates")}</caption><thead><tr><th>{t("Method")}</th><th>{t("Effective pace")}</th><th>{t("Reach")}</th><th>{t("Peak acceleration")}</th><th>{t("Peak jerk")}</th></tr></thead>
        <tbody>{preview.candidates.map(candidate=><tr key={candidate.method}>
          <td>{methodLabel(candidate.method)}</td>
          <td>{number(candidate.perceptual.pace.effective_percent)}%</td><td>{number(candidate.perceptual.position_min_percent)}–{number(candidate.perceptual.position_max_percent)}</td><td>{number(candidate.maximum_acceleration)}</td><td>{number(candidate.maximum_jerk)}</td>
        </tr>)}</tbody></table></div>

      <PatternPlotAxes durationMs={durationMs} values={[velocityValue(velocityMax),<span className="pattern-plot-value">0</span>,velocityValue(-velocityMax)]}><svg className="motion-lab-chart" viewBox="0 0 700 200" preserveAspectRatio="none" role="img" aria-label={t("Velocity estimate")}><line x1={0} x2={700} y1={100} y2={100}/>
        {baseline&&<polyline className="motion-lab-baseline" points={path(baseline,true)}/>}<polyline className="motion-lab-selected" points={path(selected,true)}/></svg></PatternPlotAxes>

    </details>
  </div>;
}
