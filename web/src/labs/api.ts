import { request } from "../api/client";
import type { MotionLabCandidate, MotionLabRequest } from "../api/motion-lab";
import type { MotionSettings } from "../api/types";

export interface FlowStep { min_percent: number; max_percent: number; speed_percent: number; cycles: number }
export interface FlowLayer { axis: "range" | "center" | "pace"; amount_percent: number; period_cycles: number; phase_percent: number; shape?: "wave" | "drift" | "alternate" }
export interface FlowSpec {
  min_percent: number; max_percent: number; speed_percent: number; range_floor_percent: number;
  anchor_percent: number; memory_cycles: number; pace_variation_percent: number; seed: number;
  steps?: FlowStep[]; layers?: FlowLayer[];
  range_ceiling_percent?: number; loop_cycles?: number;
  variation_mode?: "waves"|"drift"; turn_softness_percent?:number; cadence_hold_percent?:number;
}
export interface FlowCandidate extends Omit<MotionLabCandidate, "method"> { method: string; flow?: FlowSpec }
export interface FlowPreview { spec: FlowSpec; settings: MotionSettings; settings_key: string; candidates: FlowCandidate[] }
export interface LabTrial {
	  autopilot?:boolean; motion_applied?:boolean; motion_error?:string;
  message: string; reply: string; raw: string; error?: string; valid: boolean; changed: string[];
  model: string; method: string; prompt: string; elapsed_ms: number; provider_calls: number; schema_guided?: boolean; before: FlowSpec; after: FlowSpec;
  recipe_name?: string; limits?: MotionSettings;
}
export interface LLMLabState {
	  session?:LabSession;
  current: FlowSpec; turns: LabTrial[]; revision: number; busy: boolean; prompts: Record<string,string>;
  model: string; settings_key: string; limits: MotionSettings;
}
export interface LabSession {
  active:boolean; live:boolean; autopilot:boolean; method:string; prompt:string; model:string;
  schema_guided:boolean; interval_seconds:number; error?:string;
}
export interface ObservationTarget {
  source:"motion"|"llm"; settings_key:string; method?:string; spec?:FlowSpec; revision?:number; turn_index?:number;
}
export interface LabObservation {
  id:string; created_at:string; text:string; source:"motion"|"llm"; label:string; method:string;
  spec:FlowSpec; settings:MotionSettings; trial?:LabTrial;
}
export interface LabObservations {observations:LabObservation[]; storage_path:string; capacity:number}
export const initialFlow: FlowSpec = { min_percent: 5, max_percent: 95, speed_percent: 25, range_floor_percent: 25,
  anchor_percent: 0, memory_cycles: 8, pace_variation_percent: 10, seed: 17 };

export const labApi = {
  observations: () => request<LabObservations>("GET", "/api/labs/observations"),
  saveObservation: (target:ObservationTarget,text:string) => request<LabObservations>("POST", "/api/labs/observations", {...target,text}),
  deleteObservation: (id:string) => request<LabObservations>("DELETE", `/api/labs/observations/${encodeURIComponent(id)}`),
  preview: (spec: FlowSpec, signal?: AbortSignal) => request<FlowPreview>("POST", "/api/motion/lab/flow", spec, signal),
  state: () => request<LLMLabState>("GET", "/api/labs/llm"),
  status: () => request<Pick<LLMLabState,"revision"|"busy"|"session">>("GET", "/api/labs/llm/status"),
  session: (body:Omit<LabSession,"active">) => request<LLMLabState>("POST", "/api/labs/llm/session", body),
  chat: (body: {message:string;method:string;prompt:string;model:string;revision:number;schema_guided:boolean}, signal?: AbortSignal) => request<LLMLabState>("POST", "/api/labs/llm/chat", body, signal),
  reset: (spec?: FlowSpec, method?:string) => request<LLMLabState>("POST", "/api/labs/llm/reset", {spec,method}),
  start: (preview: FlowPreview, candidate: FlowCandidate) => {
    if (candidate.flow) return request("POST", "/api/motion/start", {lab:{method:"flow",flow:candidate.flow,settings_key:preview.settings_key}});
    const spec = preview.spec;
    const legacy: MotionLabRequest = {speed_percent:spec.speed_percent,center_percent:Math.floor((spec.min_percent+spec.max_percent)/2),
      span_percent:spec.max_percent-spec.min_percent,span_min_percent:Math.max(20,spec.range_floor_percent),span_profile:"wander",variation_percent:30,
      range_anchor_percent:spec.anchor_percent,outbound_time_percent:50,seed:spec.seed};
    return request("POST", "/api/motion/start", {lab:{method:candidate.method,request:legacy,settings_key:preview.settings_key}});
  },
};

export function exportLabReport(name: string, data: unknown) {
  const url = URL.createObjectURL(new Blob([JSON.stringify(data,null,2)], {type:"application/json"}));
  const link=document.createElement("a"); link.href=url; link.download=name; link.click();
  window.setTimeout(()=>URL.revokeObjectURL(url),0);
}
