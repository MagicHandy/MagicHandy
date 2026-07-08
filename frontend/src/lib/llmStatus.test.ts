import { describe, expect, it } from "vitest";
import {
  isOllamaProvider,
  llmBaseURLFromSnap,
  llmConnectedFromSnap,
  llmIdleFromSnap,
  llmProviderFromSnap,
} from "./llmStatus";

describe("llmStatus", () => {
  it("defaults to llama_cpp provider", () => {
    expect(llmProviderFromSnap({})).toBe("llama_cpp");
    expect(isOllamaProvider("llama_cpp")).toBe(false);
  });

  it("reads nested llm payload from status", () => {
    const snap = {
      llm_provider: "llama_cpp",
      llm_connected: true,
      llm_base_url: "http://127.0.0.1:8080",
      llm: { provider: "llama_cpp", base_url: "http://127.0.0.1:8080" },
    };
    expect(llmProviderFromSnap(snap)).toBe("llama_cpp");
    expect(llmConnectedFromSnap(snap)).toBe(true);
    expect(llmBaseURLFromSnap(snap)).toBe("http://127.0.0.1:8080");
  });

  it("treats managed llama as idle when not loaded", () => {
    expect(
      llmIdleFromSnap({
        llm_cpp_mode: "managed",
        llm: { managed: true, loaded: false },
      }),
    ).toBe(true);
    expect(
      llmIdleFromSnap({
        llm_cpp_mode: "managed",
        llm: { managed: true, loaded: true },
      }),
    ).toBe(false);
  });
});
