import type {FlowPreview,FlowSpec,LLMLabState} from "./api";
import {initialFlow} from "./api";

export const labLimits={speed_min_percent:10,speed_max_percent:43,stroke_min_percent:0,stroke_max_percent:100,reverse_direction:false,apply_video_speed_limit:false,style:"balanced" as const,handy_model:"handy_original" as const};
export function labPreview(spec:FlowSpec):FlowPreview {
  return {spec,settings:labLimits,settings_key:"saved-limits",candidates:["creative","anchored","flow"].map(method=>({
    method,flow:method==="flow"?spec:undefined,target:{source:"motion_lab",speed_percent:spec.speed_percent},period_ms:60000,
    outbound_time_percent:50,maximum_acceleration:2000,maximum_jerk:15000,
    perceptual:{position_min_percent:spec.min_percent,position_max_percent:spec.max_percent,mean_stroke_percent:50,minimum_local_stroke_cv:.2,
      minimum_local_stroke_range_percent:30,commanded_peak_velocity_percent_per_second:180,pace:{requested_percent:spec.speed_percent,effective_percent:24,limited:false,
        requested_mean_travel_percent_per_second:110,commanded_mean_travel_percent_per_second:106,commanded_peak_velocity_percent_per_second:180,device_peak_velocity_percent_per_second:364}},
    samples:[{time_ms:0,position_percent:5,velocity_percent_per_second:0},{time_ms:1000,position_percent:95,velocity_percent_per_second:0}],
  }))};
}
export function labState():LLMLabState {return {current:{...initialFlow},turns:[],revision:0,busy:false,prompts:{controls:"control-prompt",sequence:"sequence-prompt",layers:"layer-prompt",library:"library-prompt",library_descriptive:"descriptive-prompt",library_actions:"actions-prompt"},model:"local-model",settings_key:"saved-limits",limits:labLimits};}
