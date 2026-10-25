import { fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { LLMSettings, ModelCapabilityOption } from "../api/llm-settings-types";
import { modelSettingsPatch, modelSettingsSignature } from "../util/model-connections";
import { ModelCapabilityField } from "./ModelCapabilityField";

const options: ModelCapabilityOption[] = [
  { level: "simple", minimum_context: 16384, minimum_output: 64 },
  { level: "intermediate", minimum_context: 32768, minimum_output: 512 },
  { level: "advanced", minimum_context: 65536, minimum_output: 1024 },
];
const initial = { provider: "ollama", model: "model", llama_cpp_context_size: 16384, max_output_tokens: 256 } as LLMSettings;
const sizes = [16384, 32768, 65536, 131072];

function Field() {
  const [settings, setSettings] = useState(initial);
  return <><ModelCapabilityField settings={settings} options={options} contextSizes={sizes} locked={false}
    patch={change => setSettings(current => ({ ...current, ...change }))} />
    <output data-testid="draft">{JSON.stringify(settings)}</output></>;
}

describe("local model capability", () => {
  it("preserves Simple until selected, then applies server-provided floors without rerouting", () => {
    render(<Field />);
    const selector = screen.getByRole("combobox", { name: "Model capability" });
    expect(selector).toHaveValue("simple");
    expect(screen.queryByRole("combobox", { name: "Context size" })).not.toBeInTheDocument();
    fireEvent.change(selector, { target: { value: "intermediate" } });
    expect(screen.getByRole("combobox", { name: "Context size" })).toHaveValue("32768");
    expect(JSON.parse(screen.getByTestId("draft").textContent!)).toMatchObject({ provider: "ollama", model: "model", max_output_tokens: 512, local_capability_level: "intermediate" });
    fireEvent.change(selector, { target: { value: "advanced" } });
    const context = screen.getByRole("combobox", { name: "Context size" });
    expect(context).toHaveValue("65536");
    expect(within(context).queryByRole("option", { name: "32768 tokens" })).not.toBeInTheDocument();
    expect(screen.getByText(/The same motion tools as external models/)).toBeInTheDocument();
    fireEvent.change(context, { target: { value: "131072" } });
    fireEvent.change(selector, { target: { value: "simple" } });
    expect(JSON.parse(screen.getByTestId("draft").textContent!)).toMatchObject({ llama_cpp_context_size: 131072, max_output_tokens: 1024, local_capability_level: "simple" });
  });
  it("distinguishes server allocation from the application budget and respects read-only access", () => {
    const patch = vi.fn();
    render(<ModelCapabilityField settings={{ ...initial, provider: "llama_cpp", llama_cpp_mode: "external", local_capability_level: "advanced", llama_cpp_context_size: 65536 }}
      options={options} contextSizes={sizes} locked patch={patch} />);
    expect(screen.getByRole("combobox", { name: "Model capability" })).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Context size" })).toBeDisabled();
    expect(screen.getByText(/Configure your server to provide this context size/)).toBeInTheDocument();
    expect(patch).not.toHaveBeenCalled();
  });
  it("includes capability in draft save/discard and readiness invalidation", () => {
    const advanced = { ...initial, local_capability_level: "advanced" as const };
    expect(modelSettingsSignature(advanced)).not.toEqual(modelSettingsSignature(initial));
    expect(modelSettingsPatch(advanced).local_capability_level).toBe("advanced");
    expect(modelSettingsSignature({ ...initial, local_capability_level: "simple" })).toEqual(modelSettingsSignature(initial));
  });
});
