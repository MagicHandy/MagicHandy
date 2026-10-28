import type { LLMMotionCapabilities, LLMUserAnatomy } from './types';

export type ModelCapabilityLevel = "simple" | "intermediate" | "advanced";
export interface ModelCapabilityOption { level: ModelCapabilityLevel; minimum_context: number; minimum_output: number }

// Mirrors the backend model settings; private credentials have no API field.
export interface LLMSettings {
	  motion_planner?: import("./cloud-types").MotionPlannerSettings;
	  motion_reviewer?: { enabled: boolean; connection_id?: string };
	  connections?: import("./cloud-types").ModelConnection[];
	  conversation_connection_id?: string;
	  retry_refusal_locally?: boolean;
	  hosted_response_repair_enabled?: boolean;
    provider: string;
    llama_cpp_mode: string;
    managed_load_policy?: "startup" | "on_demand" | string;
    llama_cpp_base_url: string;
    llama_cpp_context_size: number;
    local_capability_level?: ModelCapabilityLevel;
    ollama_base_url: string;
    ollama_models_path?: string;
    model: string;
    prompt_set: string;
    request_timeout_ms: number;
    max_output_tokens: number;
    reasoning_mode: string;
    chat_voice?: string;
    reply_length?: string;
    user_anatomy?: LLMUserAnatomy;
    custom_anatomy?: string;
    persona_description?: string;
    motion_capabilities?: LLMMotionCapabilities;
		motion_generation_mode: "dynamic" | "pattern" | "layered" | "creative_v2" | "off" | string;
		/** Offers the retired original Creative ("dynamic") mode again (ADR 0062). */
		original_creative_mode?: boolean;
}
