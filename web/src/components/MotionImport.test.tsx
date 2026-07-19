import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MotionImport } from "./MotionImport";

function funscriptFile(actions: Array<{ at: number; pos: number }>, name = "session.funscript", extra: Record<string, unknown> = {}) {
  return new File([JSON.stringify({ ...extra, actions })], name, { type: "application/json" });
}

function pickFile(file: File) {
  const input = document.querySelector("input[type=file]");
  expect(input).not.toBeNull();
  fireEvent.change(input as HTMLInputElement, { target: { files: [file] } });
}

function readFileText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error);
    reader.readAsText(file);
  });
}

async function findTimeline(): Promise<HTMLDivElement> {
  return await screen.findByRole("group", { name: /funscript timeline editor/i }) as HTMLDivElement;
}

function setTimelineRect(timeline: HTMLDivElement, width: number) {
  const rect = {
    left: 0, right: width, top: 0, bottom: 140, width, height: 140, x: 0, y: 0, toJSON: () => ({}),
  };
  timeline.getBoundingClientRect = () => rect;
  const plot = timeline.querySelector("svg");
  expect(plot).not.toBeNull();
  if (plot) plot.getBoundingClientRect = () => rect;
}

function setScrollbarRects(scrollbar: HTMLElement, width: number, thumbLeft: number, thumbWidth: number) {
  const rect = (left: number, elementWidth: number) => ({
    left,
    right: left + elementWidth,
    top: 0,
    bottom: 32,
    width: elementWidth,
    height: 32,
    x: left,
    y: 0,
    toJSON: () => ({}),
  });
  scrollbar.getBoundingClientRect = () => rect(0, width);
  const thumb = scrollbar.querySelector(".import-timeline-scrollbar-thumb");
  expect(thumb).not.toBeNull();
  if (thumb) thumb.getBoundingClientRect = () => rect(thumbLeft, thumbWidth);
}

describe("MotionImport", () => {
  it("trims a funscript and submits the rebased selection with the chosen kind and name", async () => {
    const onImport = vi.fn().mockResolvedValue(true);
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);

    pickFile(funscriptFile([
      { at: 1000, pos: 10 },
      { at: 2000, pos: 90 },
      { at: 3000, pos: 20 },
      { at: 4000, pos: 80 },
      { at: 5000, pos: 30 },
    ]));

    // Parsed timeline rebases to 0..4000ms and defaults to the full selection.
    expect(await findTimeline()).toBeInTheDocument();
    expect(screen.getByText("5 of 5 actions selected")).toBeInTheDocument();
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:04");

    fireEvent.keyDown(screen.getByRole("slider", { name: "Trim start" }), { key: "ArrowRight" });
    fireEvent.keyDown(screen.getByRole("slider", { name: "Trim end" }), { key: "ArrowLeft" });
    expect(screen.getByText("3 of 5 actions selected")).toBeInTheDocument();
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:02");

    fireEvent.click(screen.getByRole("button", { name: "Loop pattern" }));
    fireEvent.change(screen.getByLabelText("Save as"), { target: { value: "Warmup slice" } });
    fireEvent.click(screen.getByRole("button", { name: "Import as loop pattern" }));

    await waitFor(() => expect(onImport).toHaveBeenCalledTimes(1));
    const [file, kind] = onImport.mock.calls[0] as [File, string];
    expect(kind).toBe("pattern");
    expect(file.name).toBe("Warmup slice.funscript");
    expect(JSON.parse(await readFileText(file))).toEqual({
      actions: [
        { at: 0, pos: 90 },
        { at: 1000, pos: 20 },
        { at: 2000, pos: 80 },
      ],
    });

    // A successful import clears the studio back to the pick-a-file state.
    expect(await screen.findByText("No file selected")).toBeInTheDocument();
  });

  it("snaps trim bounds to source actions so the visible length matches the imported duration", async () => {
    const onImport = vi.fn().mockResolvedValue(true);
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);

    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 1000, pos: 40 },
      { at: 2000, pos: 80 },
      { at: 3000, pos: 20 },
    ]));
    const timeline = await findTimeline();
    setTimelineRect(timeline, 300);

    const startHandle = screen.getByRole("slider", { name: "Trim start" });
    const endHandle = screen.getByRole("slider", { name: "Trim end" });
    fireEvent.pointerDown(startHandle, { button: 0, clientX: 0, pointerId: 1 });
    fireEvent.pointerMove(startHandle, { clientX: 60, pointerId: 1 });
    fireEvent.pointerUp(startHandle, { clientX: 60, pointerId: 1 });
    fireEvent.pointerDown(endHandle, { button: 0, clientX: 300, pointerId: 2 });
    fireEvent.pointerMove(endHandle, { clientX: 240, pointerId: 2 });
    fireEvent.pointerUp(endHandle, { clientX: 240, pointerId: 2 });

    expect(screen.getByRole("slider", { name: "Trim start" })).toHaveAttribute("aria-valuenow", "1000");
    expect(screen.getByRole("slider", { name: "Trim end" })).toHaveAttribute("aria-valuenow", "2000");
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:01");

    fireEvent.click(screen.getByRole("button", { name: "Import as program" }));
    await waitFor(() => expect(onImport).toHaveBeenCalledTimes(1));
    const [file] = onImport.mock.calls[0] as [File, string];
    expect(JSON.parse(await readFileText(file))).toEqual({
      actions: [
        { at: 0, pos: 40 },
        { at: 1000, pos: 80 },
      ],
    });
  });

  it("uses nearest-action pointer snapping and action-by-action keyboard changes", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 1000, pos: 40 },
      { at: 2000, pos: 80 },
      { at: 3000, pos: 20 },
    ]));
    const timeline = await findTimeline();
    setTimelineRect(timeline, 300);

    fireEvent.pointerDown(timeline, { button: 0, clientX: 180, pointerId: 1 });
    fireEvent.pointerUp(timeline, { clientX: 180, pointerId: 1 });
    expect(screen.getByRole("slider", { name: "Trim start" })).toHaveAttribute("aria-valuenow", "0");

    const startHandle = screen.getByRole("slider", { name: "Trim start" });
    fireEvent.pointerDown(startHandle, { button: 0, clientX: 0, pointerId: 2 });
    fireEvent.pointerMove(startHandle, { clientX: 90, pointerId: 2 });
    fireEvent.pointerUp(startHandle, { clientX: 90, pointerId: 2 });
    expect(startHandle).toHaveAttribute("aria-valuenow", "1000");
    expect(startHandle).toHaveAttribute("aria-valuemax", "2000");
    expect(startHandle).toHaveAttribute("aria-valuetext", "00:01");

    fireEvent.keyDown(startHandle, { key: "ArrowRight" });
    expect(startHandle).toHaveAttribute("aria-valuenow", "2000");
    expect(screen.getByRole("slider", { name: "Trim end" })).toHaveAttribute("aria-valuemin", "3000");
  });

  it("selects the nearest bound when fixed-size trim targets overlap", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 2900, pos: 80 },
      { at: 3000, pos: 20 },
    ]));
    const timeline = await findTimeline();
    setTimelineRect(timeline, 300);

    const startHandle = screen.getByRole("slider", { name: "Trim start" });
    const endHandle = screen.getByRole("slider", { name: "Trim end" });
    fireEvent.keyDown(startHandle, { key: "ArrowRight" });
    expect(startHandle).toHaveAttribute("aria-valuenow", "2900");

    // The end target paints over the narrow selection, but pointer proximity
    // still chooses the visible start boundary instead of the DOM paint order.
    fireEvent.pointerDown(endHandle, { button: 0, clientX: 291, pointerId: 1 });
    fireEvent.pointerMove(endHandle, { clientX: 0, pointerId: 1 });
    fireEvent.pointerUp(endHandle, { clientX: 0, pointerId: 1 });
    expect(startHandle).toHaveAttribute("aria-valuenow", "0");
    expect(endHandle).toHaveAttribute("aria-valuenow", "3000");
  });

  it("does not trap touch scrolling or change trim after controls become locked", async () => {
    const { rerender } = render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 1000, pos: 40 },
      { at: 2000, pos: 80 },
    ]));
    const timeline = await findTimeline();
    setTimelineRect(timeline, 200);

    fireEvent.pointerDown(timeline, { button: 0, clientX: 80, pointerId: 1, pointerType: "touch" });
    expect(screen.getByRole("slider", { name: "Trim start" })).toHaveAttribute("aria-valuenow", "0");

    fireEvent.pointerDown(screen.getByRole("slider", { name: "Trim start" }), { button: 0, clientX: 0, pointerId: 2 });
    rerender(<MotionImport locked importing={false} onImport={vi.fn()} />);
    fireEvent.pointerMove(screen.getByRole("slider", { name: "Trim start", hidden: true }), { clientX: 100, pointerId: 2 });
    fireEvent.keyDown(screen.getByRole("slider", { name: "Trim start", hidden: true }), { key: "ArrowRight" });
    expect(screen.getByRole("slider", { name: "Trim start", hidden: true })).toHaveAttribute("aria-valuenow", "0");
  });

  it("provides cursor-anchored wheel zoom, compact viewport controls, and aligned trim geometry", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 1000, pos: 100 },
      { at: 2000, pos: 0 },
      { at: 3000, pos: 100 },
      { at: 4000, pos: 0 },
    ]));
    const timeline = await findTimeline();
    setTimelineRect(timeline, 400);

    fireEvent.keyDown(screen.getByRole("slider", { name: "Trim start" }), { key: "ArrowRight" });
    fireEvent.keyDown(screen.getByRole("slider", { name: "Trim end" }), { key: "ArrowLeft" });
    expect(screen.getByRole("button", { name: "Zoom in" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Fit selection" })).toBeEnabled();

    const fullView = screen.getByLabelText("Visible timeline range").textContent;
    const viewportScrollbar = screen.getByRole("scrollbar", { name: "Timeline viewport" });
    expect(viewportScrollbar).toHaveAttribute("aria-disabled", "true");
    expect(fireEvent.wheel(timeline, { clientX: 100, deltaX: 0, deltaY: 100, deltaMode: 0 })).toBe(true);
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent(fullView ?? "");

    expect(fireEvent.wheel(timeline, { clientX: 100, deltaX: 0, deltaY: -100, deltaMode: 0 })).toBe(false);
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:00.251-00:03.248 at 1.3x");
    expect(viewportScrollbar).not.toHaveAttribute("aria-disabled");
    expect(screen.getByRole("slider", { name: "Trim start" })).toHaveAttribute("aria-valuenow", "1000");
    expect(screen.getByRole("slider", { name: "Trim end" })).toHaveAttribute("aria-valuenow", "3000");

    fireEvent.click(screen.getByRole("button", { name: "Fit selection" }));
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:01-00:03 at 2x");
    expect(screen.getByRole("slider", { name: "Trim start" })).toHaveAttribute("aria-valuenow", "1000");
    expect(screen.getByRole("slider", { name: "Trim end" })).toHaveAttribute("aria-valuenow", "3000");
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:02");

    expect(screen.getByRole("button", { name: "Fit selection" })).toBeDisabled();
    fireEvent.wheel(timeline, { deltaX: 100, deltaY: 0, deltaMode: 0 });
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:01.500-00:03.500 at 2x");

    fireEvent.click(screen.getByRole("button", { name: "Fit all" }));
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:00-00:04 at 1x");
    const dimStart = timeline.querySelector('[data-trim-dim="start"]');
    const startHandle = screen.getByRole("slider", { name: "Trim start" });
    expect(startHandle).toHaveStyle({ "--handle-position": "25%" });
    expect(Number(dimStart?.getAttribute("width"))).toBeCloseTo(190, 6);
  });

  it("moves the visible range with a proportional pointer- and keyboard-operable scrollbar", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 1000, pos: 100 },
      { at: 2000, pos: 0 },
      { at: 3000, pos: 100 },
      { at: 4000, pos: 0 },
    ]));
    await findTimeline();
    fireEvent.click(screen.getByRole("button", { name: "Zoom in" }));

    const scrollbar = screen.getByRole("scrollbar", { name: "Timeline viewport" });
    expect(scrollbar).toHaveAttribute("aria-valuemin", "0");
    expect(scrollbar).toHaveAttribute("aria-valuemax", "2000");
    expect(scrollbar).toHaveAttribute("aria-valuenow", "1000");
    expect(scrollbar).toHaveAttribute("aria-valuetext", "00:01 to 00:03");
    setScrollbarRects(scrollbar, 400, 100, 200);

    const thumb = scrollbar.querySelector(".import-timeline-scrollbar-thumb");
    expect(thumb).not.toBeNull();
    fireEvent.pointerDown(thumb as Element, { button: 0, clientX: 150, pointerId: 1 });
    fireEvent.pointerMove(scrollbar, { clientX: 250, pointerId: 1 });
    fireEvent.pointerUp(scrollbar, { clientX: 250, pointerId: 1 });
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:02-00:04 at 2x");

    fireEvent.keyDown(scrollbar, { key: "Home" });
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:00-00:02 at 2x");
    fireEvent.keyDown(scrollbar, { key: "ArrowRight" });
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:00.200-00:02.200 at 2x");
    fireEvent.keyDown(scrollbar, { key: "End" });
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:02-00:04 at 2x");
    fireEvent.pointerDown(scrollbar, { button: 0, clientX: 100, pointerId: 2 });
    fireEvent.pointerUp(scrollbar, { clientX: 100, pointerId: 2 });
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:00-00:02 at 2x");
  });

  it("maps trim dragging through the zoomed viewport before building the import payload", async () => {
    const onImport = vi.fn().mockResolvedValue(true);
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 1000, pos: 25 },
      { at: 2000, pos: 50 },
      { at: 3000, pos: 75 },
      { at: 4000, pos: 100 },
    ]));
    const timeline = await findTimeline();
    setTimelineRect(timeline, 400);

    const startHandle = screen.getByRole("slider", { name: "Trim start" });
    const endHandle = screen.getByRole("slider", { name: "Trim end" });
    fireEvent.keyDown(startHandle, { key: "ArrowRight" });
    fireEvent.keyDown(endHandle, { key: "ArrowLeft" });
    fireEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:01-00:03 at 2x");

    fireEvent.pointerDown(startHandle, { button: 0, clientX: 0, pointerId: 1 });
    fireEvent.pointerMove(startHandle, { clientX: 200, pointerId: 1 });
    fireEvent.pointerUp(startHandle, { clientX: 200, pointerId: 1 });
    expect(startHandle).toHaveAttribute("aria-valuenow", "2000");
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:01");

    fireEvent.click(screen.getByRole("button", { name: "Import as program" }));
    await waitFor(() => expect(onImport).toHaveBeenCalledTimes(1));
    const [file] = onImport.mock.calls[0] as [File, string];
    expect(JSON.parse(await readFileText(file))).toEqual({
      actions: [
        { at: 0, pos: 50 },
        { at: 1000, pos: 75 },
      ],
    });
  });

  it("keeps subsecond selection lengths visible on long files", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 250, pos: 100 },
      { at: 3600000, pos: 0 },
    ]));
    await findTimeline();

    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 1:00:00");
    fireEvent.keyDown(screen.getByRole("slider", { name: "Trim end" }), { key: "ArrowLeft" });
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:00.250");
  });

  it("allows a large source funscript to be trimmed to the import action limit", async () => {
    const onImport = vi.fn().mockResolvedValue(true);
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);
    const actions = Array.from({ length: 5000 }, (_, at) => ({ at, pos: at % 101 }));
    pickFile(funscriptFile(actions));
    const timeline = await findTimeline();
    setTimelineRect(timeline, 500);

    expect(screen.getByText(/Selection has 5000 actions/)).toBeInTheDocument();
    const endHandle = screen.getByRole("slider", { name: "Trim end" });
    fireEvent.pointerDown(endHandle, { button: 0, clientX: 500, pointerId: 1 });
    fireEvent.pointerMove(endHandle, { clientX: 100, pointerId: 1 });
    fireEvent.pointerUp(endHandle, { clientX: 100, pointerId: 1 });
    expect(screen.getByText("1001 of 5000 actions selected")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Import as program" }));

    await waitFor(() => expect(onImport).toHaveBeenCalledTimes(1));
    const [file] = onImport.mock.calls[0] as [File, string];
    expect(JSON.parse(await readFileText(file)).actions).toHaveLength(1001);
  });

  it("imports MagicHandy share files as-is with their own kind and no trim controls", async () => {
    const onImport = vi.fn().mockResolvedValue(true);
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);

    const share = new File(
      [JSON.stringify({ schema: "magichandy.pattern.v1", name: "Shared wave", kind: "routine", cycle_ms: 4000, points: [] })],
      "shared-wave.mhpattern.json",
      { type: "application/json" },
    );
    pickFile(share);

    expect(await screen.findByText("MagicHandy share file")).toBeInTheDocument();
    expect(screen.queryByRole("slider", { name: "Trim start" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Import pattern" }));

    await waitFor(() => expect(onImport).toHaveBeenCalledWith(share, "pattern"));
  });

  it("blocks loop-pattern import when the selection has no usable motion span", async () => {
    const onImport = vi.fn().mockResolvedValue(true);
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);

    pickFile(funscriptFile([
      { at: 0, pos: 50 },
      { at: 1000, pos: 50 },
      { at: 2000, pos: 50 },
    ]));

    await findTimeline();
    fireEvent.click(screen.getByRole("button", { name: "Loop pattern" }));

    expect(screen.getByText("This selection has no usable motion span for a loop pattern.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import as loop pattern" })).toBeDisabled();
    // The same flat selection is still importable as a program; the backend owns that judgement.
    fireEvent.click(screen.getByRole("button", { name: "Program" }));
    expect(screen.getByRole("button", { name: "Import as program" })).toBeEnabled();
  });

  it("preserves valid loop selections longer than the 6.6-second routine floor", async () => {
    const onImport = vi.fn().mockResolvedValue(true);
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);
    pickFile(funscriptFile([
      { at: 0, pos: 20 },
      { at: 3000, pos: 80 },
      { at: 6000, pos: 20 },
      { at: 9000, pos: 80 },
      { at: 12000, pos: 20 },
    ]));

    await findTimeline();
    fireEvent.click(screen.getByRole("button", { name: "Loop pattern" }));
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:12");
    expect(screen.getByText(/Active timing remains as selected/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Import as loop pattern" }));

    await waitFor(() => expect(onImport).toHaveBeenCalledTimes(1));
    const [file, kind] = onImport.mock.calls[0] as [File, string];
    expect(kind).toBe("pattern");
    const actions = (JSON.parse(await readFileText(file)) as { actions: Array<{ at: number }> }).actions;
    expect(actions[actions.length - 1].at).toBe(12000);
  });

  it("blocks loop selections that exceed the backend essential-knot limit", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile(Array.from({ length: 260 }, (_, index) => ({
      at: index * 50,
      pos: index % 2 === 0 ? 0 : 100,
    }))));

    await findTimeline();
    fireEvent.click(screen.getByRole("button", { name: "Loop pattern" }));
    expect(screen.getByText("This loop has 260 essential reversal knots; trim to a simpler section with 255 or fewer."))
      .toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import as loop pattern" })).toBeDisabled();
  });

  it("reports unusable files instead of submitting them", async () => {
    const onImport = vi.fn();
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);

    pickFile(new File(["not json"], "broken.funscript", { type: "application/json" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("broken.funscript is not valid JSON.");
    expect(onImport).not.toHaveBeenCalled();
  });

  it("rejects malformed funscript actions instead of repairing them", async () => {
    const onImport = vi.fn();
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);

    pickFile(new File([JSON.stringify({ actions: [{ at: 0, pos: 120 }, { at: 1000, pos: 50 }] })], "unsafe.funscript"));
    expect(await screen.findByRole("alert")).toHaveTextContent("unsafe.funscript action 1 position must be between 0 and 100.");

    pickFile(new File([JSON.stringify({ schema: "other.motion.v1", actions: [{ at: 0, pos: 0 }, { at: 1000, pos: 100 }] })], "unknown.json"));
    expect(await screen.findByRole("alert")).toHaveTextContent("unknown.json uses an unknown motion content schema.");

    pickFile(new File([JSON.stringify({ schema: "", actions: [{ at: 0, pos: 0 }, { at: 1000, pos: 100 }] })], "empty-schema.json"));
    expect(await screen.findByRole("alert")).toHaveTextContent("empty-schema.json uses an unknown motion content schema.");

    pickFile(new File([JSON.stringify({ inverted: "true", actions: [{ at: 0, pos: 0 }, { at: 1000, pos: 100 }] })], "inverted.funscript"));
    expect(await screen.findByRole("alert")).toHaveTextContent("inverted.funscript has an invalid inverted flag.");
    expect(onImport).not.toHaveBeenCalled();
  });

  it("rejects save names that the backend would interpret as a path", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([{ at: 0, pos: 0 }, { at: 1000, pos: 100 }]));
    await findTimeline();

    fireEvent.change(screen.getByLabelText("Save as"), { target: { value: "Warmup / Finale" } });
    expect(screen.getByText("Name cannot contain path separators (/ or \\).")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import as program" })).toBeDisabled();
  });
});
