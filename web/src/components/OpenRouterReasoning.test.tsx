import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { newModelConnection, connectionSignature } from "../util/model-connections";
import { ModelResponseSettings, selectedModelChange } from "./ModelResponseSettings";

const connection = { ...newModelConnection("openrouter", []), model: "fixture" };
const model = { id: "fixture", name: "Fixture", reasoning_supported: true, reasoning_efforts: ["none", "low", "high"] };
describe("OpenRouter reasoning", () => {
 it("shows the advertised default effort without selecting an override", () => {
  const patch = vi.fn();
  render(<ModelResponseSettings connection={connection} models={[{ ...model, default_reasoning_effort: "high" }]} disabled={false} patch={patch} />);
  expect(screen.getByRole("combobox", { name: "Reasoning" })).toHaveDisplayValue("Provider default (High)");
  expect(patch).not.toHaveBeenCalled();
 });
 it("preserves Off when switching to a model with on/off but no none effort", () => {
  const budgetModel = { ...model, id: "budget-model", reasoning_efforts: [] };
  expect(selectedModelChange(budgetModel, budgetModel.id, { ...connection, reasoning_effort: "none" })).toMatchObject({
   reasoning_effort: "", reasoning_enabled: false,
  });
 });
 it("does not call a saved setting unavailable before catalogue discovery", () => {
  const patch = vi.fn();
  const saved = { ...connection, reasoning_effort: "low" };
  const { rerender } = render(<ModelResponseSettings connection={saved} models={[]} disabled={true} patch={patch} />);
  expect(screen.queryByRole("option", { name: "Saved setting unavailable — choose another" })).not.toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "Reasoning" })).toHaveDisplayValue("Low — quicker responses");
  expect(patch).not.toHaveBeenCalled();
  rerender(<ModelResponseSettings connection={saved} models={[{ ...model, reasoning_supported: false, reasoning_efforts: [] }]} disabled={false} patch={patch} />);
  expect(screen.getByRole("option", { name: "Saved setting unavailable — choose another" })).toBeDisabled();
 });
 it("uses discovered options and the advertised none effort for Off", () => {
  const patch=vi.fn();render(<ModelResponseSettings connection={connection} models={[model]} disabled={false} patch={patch} compact />);
  const select=screen.getByRole("combobox",{name:"Reasoning"});
  expect(Array.from(select.querySelectorAll("option")).map(o=>o.value)).toEqual(["","off","on","low","high"]);
  fireEvent.change(select,{target:{value:"off"}});expect(patch).toHaveBeenCalledWith({reasoning_effort:"none",reasoning_enabled:undefined});
  expect(connectionSignature(connection)).not.toBe(connectionSignature({...connection,reasoning_enabled:false}));
 });
 it("does not offer disabling mandatory reasoning and permits recovery from a stale saved setting", () => {
  render(<ModelResponseSettings connection={{...connection,reasoning_enabled:false}} models={[{...model,reasoning_mandatory:true,reasoning_efforts:["low"]}]} disabled={false} patch={vi.fn()} />);
  expect(screen.getByRole("option",{name:"Saved setting unavailable — choose another"})).toBeDisabled();
  expect(screen.queryByRole("option",{name:"Off"})).not.toBeInTheDocument();
 });
 it("keeps provider default on a new selection and resets an incompatible choice", () => {
  expect(selectedModelChange(model,"fixture",connection).reasoning_effort).toBe("");
  const next=selectedModelChange({...model,reasoning_mandatory:true},"other",{...connection,reasoning_enabled:false});
  expect(next.reasoning_enabled).toBeUndefined();expect(next.reasoning_effort).toBe("");
 });
 it("exposes on/off without inventing effort levels for budget-only models", () => {
  render(<ModelResponseSettings connection={connection} models={[{...model,reasoning_efforts:[]}]} disabled={true} patch={vi.fn()} />);
  expect(screen.getByRole("combobox")).toBeDisabled();
  expect(screen.getAllByRole("option")).toHaveLength(3);
 });
});
