import type { SetupStatus } from "../api/types";

export const setupFixture: SetupStatus = {
  required: true,
  data_dir: "C:\\Users\\Test User\\AppData\\Roaming\\MagicHandy",
  hardware: { platform: "windows/amd64", nvidia: true, cuda: false, gpu_name: "Test NVIDIA GPU", vram_mib: "8192" },
  voice_modules: [
    {
      id: "faster-qwen3-tts", name: "Faster Qwen3-TTS", provider: "faster_qwen3_tts",
      summary: "Fast local voice cloning.", license: "MIT", model: "Qwen/Qwen3-TTS-12Hz-0.6B-Base",
      model_license: "Apache-2.0", python_version: "3.11", disk_estimate: "Several GiB",
      supported_devices: ["cuda"], recommended_for_nvidia: true,
      reference_requirement: "Add a reference in Voice settings.", source_url: "https://example.invalid",
      source_revision: "fixture", port: 8991,
    },
  ],
  llama_runtime: {
    name: "Managed llama.cpp", summary: "Pinned local runner.", license: "MIT", source_version: "fixture",
    disk_estimate: "CPU 18 MiB; CUDA 628 MiB", build_dependencies: ["PowerShell", "NVIDIA driver for CUDA"], backends: ["auto", "cpu", "cuda"],
  },
  parakeet: {
    name: "Parakeet", summary: "Local speech recognition.", runner_license: "MIT", model_license: "CC-BY-4.0",
    download_size: "About 646 MiB", runner_version: "fixture", model: "TDT 0.6B v3 Q4_K",
  },
  scripts_present: true,
  helpers: { llama: true, parakeet: true, voice: true },
};
