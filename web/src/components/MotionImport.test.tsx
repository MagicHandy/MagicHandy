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

    fireEvent.change(screen.getByLabelText("Trim minimum"), { target: { value: "1000" } });
    fireEvent.change(screen.getByLabelText("Trim maximum"), { target: { value: "3000" } });
    expect(screen.getByText("3 of 5 actions selected")).toBeInTheDocument();

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
});
