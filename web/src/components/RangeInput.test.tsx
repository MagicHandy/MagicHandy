import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RangeInput } from "./RangeInput";

describe("RangeInput", () => {
  it("stays a native slider and publishes its fill for the shared paint", () => {
    const change = vi.fn();
    render(<RangeInput aria-label="Speed" min={10} max={110} value={35} onChange={change} />);

    const slider = screen.getByRole("slider", { name: "Speed" });
    expect(slider).toHaveAttribute("type", "range");
    expect(slider).toHaveValue("35");
    expect(slider.style.getPropertyValue("--range-fill")).toBe("0.25");
    fireEvent.change(slider, { target: { value: "60" } });
    expect(change).toHaveBeenCalledOnce();
  });

  it("clamps the fill and reads an empty span as empty", () => {
    const { rerender } = render(<RangeInput aria-label="Offset" min={0} max={10} value={14} onChange={vi.fn()} />);
    expect(screen.getByRole("slider").style.getPropertyValue("--range-fill")).toBe("1");
    rerender(<RangeInput aria-label="Offset" min={0} max={10} value={-3} onChange={vi.fn()} />);
    expect(screen.getByRole("slider").style.getPropertyValue("--range-fill")).toBe("0");
    rerender(<RangeInput aria-label="Offset" min={5} max={5} value={5} onChange={vi.fn()} />);
    expect(screen.getByRole("slider").style.getPropertyValue("--range-fill")).toBe("0");
  });
});
