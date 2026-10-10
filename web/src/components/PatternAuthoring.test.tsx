import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { PatternPreview } from "../api/types";
import { PatternAuthoring } from "./PatternAuthoring";

describe("PatternAuthoring", () => {
  it("keeps knot input focus when its time changes", () => {
    render(<PatternAuthoring locked={false} saving={false} onPreview={vi.fn()} onSave={vi.fn()} />);
    expect(screen.getByRole("heading", { level: 2, name: "Pattern authoring" })).toBeInTheDocument();
    fireEvent.click(screen.getByText("Edit sparse knots"));
    const firstTime = screen.getAllByLabelText("Time")[0];
    firstTime.focus();

    fireEvent.change(firstTime, { target: { value: "100" } });

    expect(firstTime).toHaveFocus();
    expect(firstTime).toHaveValue(100);
  });

  it("keeps an in-progress pointer draft separate from React state", () => {
    render(<PatternAuthoring locked={false} saving={false} onPreview={vi.fn()} onSave={vi.fn()} />);
    const canvas = screen.getByLabelText("Pattern drawing canvas") as HTMLCanvasElement;
    canvas.setPointerCapture = vi.fn();
    canvas.getBoundingClientRect = () => ({
      bottom: 100,
      height: 100,
      left: 0,
      right: 100,
      top: 0,
      width: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });

    fireEvent.pointerDown(canvas, { clientX: 10, clientY: 80, pointerId: 1 });
    fireEvent.pointerMove(canvas, { clientX: 50, clientY: 20, pointerId: 1 });
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Trigger render" } });

    const sourcePoints = screen.getByText("Source points").parentElement?.querySelector("dd");
    expect(sourcePoints).toHaveTextContent("1");
  });

  it("ignores an older preview after geometry changes launch a newer request", async () => {
    const first = deferred<PatternPreview>();
    const second = deferred<PatternPreview>();
    const onPreview = vi.fn()
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);
    const onPreviewError = vi.fn();
    render(<PatternAuthoring locked={false} saving={false} onPreview={onPreview} onPreviewError={onPreviewError} onSave={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    fireEvent.click(screen.getByText("Edit sparse knots"));
    const firstTime = screen.getAllByLabelText("Time")[0];
    fireEvent.change(firstTime, { target: { value: "100" } });
    fireEvent.blur(firstTime);
    expect(onPreview).toHaveBeenCalledTimes(2);

    await act(async () => second.resolve(preview(7000, 22)));
    await waitFor(() => expect(screen.getByLabelText("Cycle length (seconds)")).toHaveValue(7));

    await act(async () => first.reject(new Error("obsolete preview failed")));
    await waitFor(() => expect(screen.getByLabelText("Cycle length (seconds)")).toHaveValue(7));
    expect(screen.getByText("22")).toBeInTheDocument();
    expect(onPreviewError).not.toHaveBeenCalled();
  });

  it("labels the plot's scales and names the sampled line only once it exists", async () => {
    const sampled = deferred<PatternPreview>();
    const result = render(<PatternAuthoring locked={false} saving={false} onPreview={() => sampled.promise} onSave={vi.fn()} />);

    const axes = result.container.querySelector(".pattern-plot");
    expect(axes).toContainElement(screen.getByLabelText("Pattern drawing canvas"));
    expect(axes?.querySelector(".pattern-plot-y")).toHaveTextContent("Tip 10050Base 0");
    expect(axes?.querySelector(".pattern-plot-x")).toHaveTextContent("0 s3.3 s6.6 s");
    expect(screen.queryByText("Backend preview")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    await act(async () => sampled.resolve(preview(8000, 3)));
    expect(await screen.findByText("Backend preview")).toBeInTheDocument();
    expect(axes?.querySelector(".pattern-plot-x")).toHaveTextContent("0 s4 s8 s");
  });

  it("reports only the active preview failure", async () => {
    const error = new Error("preview rejected");
    const onPreviewError = vi.fn();
    render(<PatternAuthoring locked={false} saving={false} onPreview={vi.fn().mockRejectedValue(error)} onPreviewError={onPreviewError} onSave={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    await waitFor(() => expect(onPreviewError).toHaveBeenCalledWith(error));
    expect(screen.getByRole("button", { name: "Preview" })).toBeEnabled();
  });
});

function preview(cycle: number, originalCount: number): PatternPreview {
  const points = [{ time_ms: 0, position_percent: 10 }, { time_ms: cycle, position_percent: 90 }];
  return { points, samples: points, cycle_ms: cycle, original_count: originalCount, simplified_count: 2 };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}
