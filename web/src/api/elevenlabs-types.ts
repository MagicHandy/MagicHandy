// ElevenLabs account, catalog and delivery shapes (/api/voice/elevenlabs/*
// and the voice settings' elevenlabs_delivery). Keys never appear here.

// Overrides for the selected ElevenLabs voice's own settings. An absent value
// keeps the voice's setting; the worker sends only what the model honors.
export interface ElevenLabsDelivery {
  stability?: number;
  similarity?: number;
  style?: number;
  speed?: number;
  language?: string;
}

export interface ElevenLabsProbe {
  state: "ok" | "denied" | "failed";
  kind?: string;
  message?: string;
}

export interface ElevenLabsVoice {
  voice_id: string;
  name: string;
  category?: string;
  description?: string | null;
  preview_url?: string | null;
  labels?: Record<string, string> | null;
  is_legacy?: boolean;
  is_owner?: boolean | null;
}

export interface ElevenLabsAccount {
  key_set: boolean;
  key_status: "missing" | "ok" | "restricted" | "invalid" | "unusual_activity" | "unreachable" | "error";
  message?: string;
  checked_at?: string;
  permissions?: Partial<Record<"user_read" | "models_read" | "voices_read", ElevenLabsProbe>>;
  subscription?: {
    tier: string;
    status: string;
    character_count: number;
    character_limit: number;
    next_reset_unix?: number | null;
    voice_slots: number;
    voice_limit: number;
  };
  library_via_api?: boolean;
  selected_voice?: ElevenLabsVoice;
  selected_voice_problem?: string;
  selected_model_problem?: string;
}

export interface ElevenLabsModel {
  model_id: string;
  name: string;
  description?: string;
  max_text_length: number;
  cost_multiplier?: number;
  languages?: { language_id: string; name: string }[];
  requires_alpha_access?: boolean;
  supports: Record<"stability" | "similarity" | "style" | "speaker_boost" | "speed" | "language_code" | "audio_tags", boolean>;
}

export interface ElevenLabsVoicePage {
  voices: ElevenLabsVoice[] | null;
  has_more: boolean;
  next_page_token?: string;
}

export interface ElevenLabsLibraryVoice {
  public_owner_id: string;
  voice_id: string;
  name: string;
  description?: string | null;
  accent?: string;
  gender?: string;
  age?: string;
  descriptive?: string;
  use_case?: string;
  category?: string;
  language?: string | null;
  preview_url?: string | null;
  free_users_allowed: boolean;
  live_moderation_enabled: boolean;
  rate?: number | null;
  is_added_by_user?: boolean | null;
}

export interface ElevenLabsLibraryPage {
  voices: ElevenLabsLibraryVoice[] | null;
  has_more: boolean;
}
