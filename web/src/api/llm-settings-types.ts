import type { LLMMotionCapabilities, LLMUserAnatomy } from './types';

// Mirrors the backend model settings; private credentials have no API field.
export interface LLMSettings {
	  motion_planner?: import("./cloud-types").MotionPlannerSettings;
	  connections?: import("./cloud-types").ModelConnection[];
	  conversation_connection_id?: string;
	  retry_refusal_locally?: boolean;
    provider: string;
    llama_cpp_mode: string;
    managed_load_policy?: "startup" | "on_demand" | string;
    llama_cpp_base_url: string;
    llama_cpp_context_size: number;
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
}
