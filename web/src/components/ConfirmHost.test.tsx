import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { confirmThen } from "../util/confirm";
import { ConfirmHost } from "./ConfirmHost";

function Harness({ proceed }: { proceed: () => void }) {
  return (
    <>
      <button type="button" data-emergency-stop>Stop</button>
      <button type="button" onClick={() => confirmThen("Delete Clip?", { confirmLabel: "Delete", destructive: true }, proceed)}>Ask</button>
      <ConfirmHost />
    </>
  );
}

describe("ConfirmHost", () => {
  it("asks inside the app instead of blocking the page with window.confirm", () => {
    const native = vi.spyOn(window, "confirm");
    const proceed = vi.fn();
    render(<Harness proceed={proceed} />);

    fireEvent.click(screen.getByRole("button", { name: "Ask" }));
    const dialog = screen.getByRole("alertdialog", { name: "Delete Clip?" });
    expect(native).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus();

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(proceed).toHaveBeenCalledOnce();
    expect(dialog).not.toBeInTheDocument();
    native.mockRestore();
  });

  it("treats Cancel and Escape as a refusal", () => {
    const proceed = vi.fn();
    render(<Harness proceed={proceed} />);

    fireEvent.click(screen.getByRole("button", { name: "Ask" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Ask" }));
    fireEvent.keyDown(document, { key: "Escape" });

    expect(proceed).not.toHaveBeenCalled();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("keeps Emergency Stop reachable from the dialog's tab order", () => {
    render(<Harness proceed={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Ask" }));

    fireEvent.keyDown(document, { key: "Tab" });
    fireEvent.keyDown(document, { key: "Tab" });

    expect(screen.getByRole("button", { name: "Stop" })).toHaveFocus();
  });

  it("falls back to the browser dialog when no host is mounted", () => {
    const native = vi.spyOn(window, "confirm").mockReturnValue(true);
    const proceed = vi.fn();
    confirmThen("Remove?", {}, proceed);
    expect(native).toHaveBeenCalledWith("Remove?");
    expect(proceed).toHaveBeenCalledOnce();
    native.mockRestore();
  });
});
