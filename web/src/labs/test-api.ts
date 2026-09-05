import {request} from "../api/client";
import type {FlowPreview,LabObservation,ObservationTarget} from "./api";

export type ReviewBasis="preview"|"device"|"simulation"|"reply"|"skipped";
export interface TestFeedback {rating:number;basis:ReviewBasis;comment:string;created_at:string}
export interface TestStep {id:string;title:string;instruction:string;phase?:string;source:LabObservation;preview?:FlowPreview;preview_error?:string;feedback?:TestFeedback}
export interface TestRun {id:string;title:string;created_at:string;version:string;commit:string;revision:number;steps:TestStep[]}
export interface TestRunView {run:TestRun;next_index:number;can_audition:boolean;warning?:string;storage_path:string}
export interface TestRunList {runs:Array<{id:string;title:string;created_at:string;completed:number;total:number}>;storage_path:string;capacity:number}
export const testApi={
  list:()=>request<TestRunList>("GET","/api/labs/tests"),
  get:(id:string,signal?:AbortSignal)=>request<TestRunView>("GET",`/api/labs/tests/${encodeURIComponent(id)}`,undefined,signal),
  create:(preset:"motion_comparison"|"motion_experiments"|"llm_comparison",target?:ObservationTarget)=>request<TestRunView>("POST","/api/labs/tests",{preset,target}),
  feedback:(view:TestRunView,rating:number,basis:ReviewBasis,comment:string)=>request<TestRunView>("POST",`/api/labs/tests/${encodeURIComponent(view.run.id)}/feedback`,{revision:view.run.revision,step_id:view.run.steps[view.next_index].id,rating,basis,comment}),
  remove:(id:string)=>request("DELETE",`/api/labs/tests/${encodeURIComponent(id)}`),
};

export function openTestRun(view:TestRunView) {window.location.hash=`#/labs/tests/${encodeURIComponent(view.run.id)}`;}
