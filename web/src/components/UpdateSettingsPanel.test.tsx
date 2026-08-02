import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { UpdateSettingsPanel } from "./UpdateSettingsPanel";

vi.mock("../api/client", () => ({
  api: { updateStatus: vi.fn() },
}));

const updateStatus = vi.mocked(api.updateStatus);

describe("UpdateSettingsPanel", () => {
  beforeEach(() => {
    setLocaleForTest("en", english);
    updateStatus.mockReset();
  });

  it("checks on demand and links to a newer stable release", async () => {
    updateStatus.mockResolvedValue({
      state: "available",
      current_version: "1.0.0",
      latest: {
        version: "1.1.0",
        tag: "v1.1.0",
        url: "https://github.com/MagicHandy/MagicHandy/releases/tag/v1.1.0",
      },
    });
    render(<UpdateSettingsPanel currentVersion="1.0.0" automatic={false} preferenceDisabled={false} checkDisabled={false} onAutomaticChange={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Check now" }));

    expect(await screen.findByText("MagicHandy 1.1.0 is available.")).toBeInTheDocument();
    expect(updateStatus).toHaveBeenCalledWith(true);
    expect(screen.getByRole("link", { name: "View release" })).toHaveAttribute(
      "href",
      "https://github.com/MagicHandy/MagicHandy/releases/tag/v1.1.0",
    );
  });

  it("exposes automatic checking as a normal saved preference", () => {
    const onAutomaticChange = vi.fn();
    render(<UpdateSettingsPanel currentVersion="dev" automatic={false} preferenceDisabled={false} checkDisabled={false} onAutomaticChange={onAutomaticChange} />);

    fireEvent.click(screen.getByRole("checkbox", { name: "Check for updates automatically" }));

    expect(onAutomaticChange).toHaveBeenCalledWith(true);
    expect(updateStatus).not.toHaveBeenCalled();
  });

  it("shows the cached result when automatic checks are enabled", async () => {
    updateStatus.mockResolvedValue({
      state: "available",
      current_version: "1.0.0",
      latest: {
        version: "1.1.0",
        tag: "v1.1.0",
        url: "https://github.com/MagicHandy/MagicHandy/releases/tag/v1.1.0",
      },
    });

    render(<UpdateSettingsPanel currentVersion="1.0.0" automatic preferenceDisabled={false} checkDisabled={false} onAutomaticChange={vi.fn()} />);

    expect(await screen.findByText("MagicHandy 1.1.0 is available.")).toBeInTheDocument();
    expect(updateStatus).toHaveBeenCalledWith();
  });

  it("does not automatically check from an unversioned source build", () => {
    render(<UpdateSettingsPanel currentVersion="dev" automatic preferenceDisabled={false} checkDisabled={false} onAutomaticChange={vi.fn()} />);

    expect(updateStatus).not.toHaveBeenCalled();
  });

  it("keeps manual checks available when only preference writes are locked", () => {
    updateStatus.mockResolvedValue({ state: "current", current_version: "1.0.0" });
    render(<UpdateSettingsPanel currentVersion="1.0.0" automatic={false} preferenceDisabled checkDisabled={false} onAutomaticChange={vi.fn()} />);

    expect(screen.getByRole("checkbox", { name: "Check for updates automatically" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Check now" })).toBeEnabled();
  });

  it("labels a retained result as stale after refresh failure", async () => {
    updateStatus.mockResolvedValue({
      state: "current",
      current_version: "1.0.0",
      latest: { version: "1.0.0", tag: "v1.0.0", url: "https://github.com/MagicHandy/MagicHandy/releases/tag/v1.0.0" },
      stale: true,
    });
    render(<UpdateSettingsPanel currentVersion="1.0.0" automatic={false} preferenceDisabled={false} checkDisabled={false} onAutomaticChange={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Check now" }));

    expect(await screen.findByText("Showing the last successful result because GitHub could not be refreshed.")).toBeInTheDocument();
  });
});
