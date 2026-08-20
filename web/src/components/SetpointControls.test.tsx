import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SegmentedChoice, SetpointSlider } from "./SetpointControls";

describe("SetpointControls", () => {
  it("exposes an ordered scale as a named native slider", () => {
    const change = vi.fn();
    const result = render(
      <SetpointSlider
        label="Style"
        value="balanced"
        options={[
          { value: "gentle", label: "Gentle" },
          { value: "balanced", label: "Balanced" },
          { value: "intense", label: "Intense" },
        ]}
        onChange={change}
      />,
    );

    const slider = screen.getByRole("slider", { name: "Style" });
    expect(slider).toHaveValue("1");
    expect(slider).toHaveAttribute("aria-valuetext", "Balanced");
    expect(slider.style.getPropertyValue("--setpoint-progress")).toBe("50%");
    expect(Array.from(result.container.querySelectorAll<HTMLElement>(".setpoint-stop"), (stop) =>
      stop.style.getPropertyValue("--setpoint-position"),
    )).toEqual(["0%", "50%", "100%"]);
    expect(result.container.querySelector('.setpoint-stop[data-selected="true"]')).toHaveTextContent("Balanced");
    expect(screen.getByText("Balanced", { selector: "output" })).toBeInTheDocument();
    fireEvent.change(slider, { target: { value: "2" } });
    expect(change).toHaveBeenCalledWith("intense");
  });

  it("keeps categorical modes as a segmented radio choice", () => {
    const change = vi.fn();
    render(
      <SegmentedChoice
        label="LLM motion"
        value="pattern"
        options={[
          { value: "dynamic", label: "Creative" },
          { value: "pattern", label: "Pattern library" },
          { value: "off", label: "Off" },
        ]}
        onChange={change}
      />,
    );

    const group = screen.getByRole("radiogroup", { name: "LLM motion" });
    expect(within(group).getByRole("radio", { name: "Pattern library" })).toBeChecked();
    fireEvent.click(within(group).getByRole("radio", { name: "Off" }));
    expect(change).toHaveBeenCalledWith("off");
  });
});
