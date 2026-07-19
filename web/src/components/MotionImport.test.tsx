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
    expect(await screen.findByRole("img", { name: /funscript timeline/i })).toBeInTheDocument();
    expect(screen.getByText("5 of 5 actions selected")).toBeInTheDocument();
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:04");

    fireEvent.change(screen.getByLabelText("Trim minimum"), { target: { value: "1000" } });
    fireEvent.change(screen.getByLabelText("Trim maximum"), { target: { value: "3000" } });
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
    await screen.findByRole("img", { name: /funscript timeline/i });

    fireEvent.change(screen.getByLabelText("Trim minimum"), { target: { value: "600" } });
    fireEvent.change(screen.getByLabelText("Trim maximum"), { target: { value: "2400" } });

    expect(screen.getByLabelText("Trim minimum")).toHaveValue("1000");
    expect(screen.getByLabelText("Trim maximum")).toHaveValue("2000");
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
    await screen.findByRole("img", { name: /funscript timeline/i });

    const trimGroup = screen.getByRole("group", { name: "Trim" });
    const track = trimGroup.querySelector(".range-slider-track") as HTMLDivElement;
    track.getBoundingClientRect = () => ({
      left: 0, right: 300, top: 0, bottom: 28, width: 300, height: 28, x: 0, y: 0, toJSON: () => ({}),
    });

    fireEvent.pointerDown(track, { button: 0, clientX: 10, pointerId: 1 });
    fireEvent.pointerUp(track, { clientX: 10, pointerId: 1 });
    expect(screen.getByLabelText("Trim minimum")).toHaveValue("0");

    fireEvent.pointerDown(track, { button: 0, clientX: 90, pointerId: 2 });
    fireEvent.pointerUp(track, { clientX: 90, pointerId: 2 });
    const minimum = screen.getByLabelText("Trim minimum");
    expect(minimum).toHaveValue("1000");
    expect(minimum).toHaveAttribute("aria-valuemax", "2000");
    expect(minimum).toHaveAttribute("aria-valuetext", "00:01");

    fireEvent.change(minimum, { target: { value: "1001" } });
    expect(screen.getByLabelText("Trim minimum")).toHaveValue("2000");
    expect(screen.getByLabelText("Trim maximum")).toHaveAttribute("aria-valuemin", "3000");
  });

  it("zooms and pans the source timeline without changing the trim selection", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 1000, pos: 100 },
      { at: 2000, pos: 0 },
      { at: 3000, pos: 100 },
      { at: 4000, pos: 0 },
    ]));
    await screen.findByRole("img", { name: /funscript timeline/i });

    fireEvent.change(screen.getByLabelText("Trim minimum"), { target: { value: "1000" } });
    fireEvent.change(screen.getByLabelText("Trim maximum"), { target: { value: "3000" } });
    fireEvent.click(screen.getByRole("button", { name: "Fit selection" }));
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:01-00:03 at 2x");

    fireEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:01.500-00:02.500 at 4x");
    expect(screen.getByLabelText("Trim minimum")).toHaveValue("1000");
    expect(screen.getByLabelText("Trim maximum")).toHaveValue("3000");
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:02");

    fireEvent.click(screen.getByRole("button", { name: "Earlier" }));
    expect(screen.getByLabelText("Visible timeline range")).toHaveTextContent("Viewing 00:00.750-00:01.750 at 4x");
  });

  it("keeps subsecond selection lengths visible on long files", async () => {
    render(<MotionImport locked={false} importing={false} onImport={vi.fn()} />);
    pickFile(funscriptFile([
      { at: 0, pos: 0 },
      { at: 250, pos: 100 },
      { at: 3600000, pos: 0 },
    ]));
    await screen.findByRole("img", { name: /funscript timeline/i });

    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 1:00:00");
    fireEvent.change(screen.getByLabelText("Trim maximum"), { target: { value: "250" } });
    expect(screen.getByLabelText("Current trim selection length")).toHaveTextContent("Selection length 00:00.250");
  });

  it("allows a large source funscript to be trimmed to the import action limit", async () => {
    const onImport = vi.fn().mockResolvedValue(true);
    render(<MotionImport locked={false} importing={false} onImport={onImport} />);
    const actions = Array.from({ length: 5000 }, (_, at) => ({ at, pos: at % 101 }));
    pickFile(funscriptFile(actions));
    await screen.findByRole("img", { name: /funscript timeline/i });

    expect(screen.getByText(/Selection has 5000 actions/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Trim maximum"), { target: { value: "1000" } });
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
    expect(screen.queryByLabelText("Trim minimum")).not.toBeInTheDocument();
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

    await screen.findByRole("img", { name: /funscript timeline/i });
    fireEvent.click(screen.getByRole("button", { name: "Loop pattern" }));

    expect(screen.getByText("This selection has no usable motion span for a loop pattern.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import as loop pattern" })).toBeDisabled();
    // The same flat selection is still importable as a program; the backend owns that judgement.
    fireEvent.click(screen.getByRole("button", { name: "Program" }));
    expect(screen.getByRole("button", { name: "Import as program" })).toBeEnabled();
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
    await screen.findByRole("img", { name: /funscript timeline/i });

    fireEvent.change(screen.getByLabelText("Save as"), { target: { value: "Warmup / Finale" } });
    expect(screen.getByText("Name cannot contain path separators (/ or \\).")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import as program" })).toBeDisabled();
  });
});
