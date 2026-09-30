// Easy Setup's backend-authoritative verdict for this machine. Reasons are
// stable codes the browser words in the user's language.
export interface SetupRequirement {
  status: "met" | "partial" | "unmet";
  reason: string;
  // Free disk space the feature still needs, in bytes.
  bytes: number;
  vram_mib?: number;
  min_vram_mib?: number;
}

export interface SetupAssessment {
  free_disk_bytes: number;
  chat: SetupRequirement;
  voice_output: SetupRequirement;
  voice_input: SetupRequirement;
  model_id?: string;
  model_installed_id?: string;
  runtime_backend: "cpu" | "cuda";
  voice_module: string;
  voice_device: "cpu" | "cuda";
}
