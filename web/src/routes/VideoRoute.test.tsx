import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { VideoRoute } from "./VideoRoute";

const app = vi.hoisted(() => ({ backendOnline: true, readOnly: false, state: { stop_sequence: 7 } }));

vi.mock("../state/app-state", () => ({
  useAppState: () => app,
}));

vi.mock("../components/VideoLibrary", () => ({
  VideoLibrary: ({ locked, stopSequence }: { locked: boolean; stopSequence?: number }) => <div data-testid="video-catalog" data-locked={locked} data-stop-sequence={stopSequence}>Catalog</div>,
}));

describe("VideoRoute", () => {
  beforeEach(() => {
    app.backendOnline = true;
    app.readOnly = false;
  });

  it("renders Videos as a dedicated wide workspace", () => {
    render(<VideoRoute />);

    expect(screen.getByRole("heading", { level: 1, name: "Videos" })).toHaveFocus();
    expect(screen.getByTestId("video-catalog")).toHaveAttribute("data-locked", "false");
    expect(screen.getByTestId("video-catalog")).toHaveAttribute("data-stop-sequence", "7");
    expect(screen.getByTestId("video-catalog").parentElement).toHaveClass("video-page");
  });

  it("locks catalog mutations for an offline or read-only client", () => {
    app.readOnly = true;
    render(<VideoRoute />);

    expect(screen.getByTestId("video-catalog")).toHaveAttribute("data-locked", "true");
  });
});
