import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ControlIdentity, UserAccount } from "../api/types";
import { setLocaleForTest } from "../i18n";
import { ControlIdentitySelector } from "./ControlIdentitySelector";

function account(id: string, username: string, profile = false): UserAccount {
  return {
    id,
    username,
    role: "operator",
    disabled: false,
    has_profile_image: profile,
    profile_updated_at: profile ? "2026-08-28T12:00:00Z" : "",
    created_at: "2026-08-28T12:00:00Z",
    updated_at: "2026-08-28T12:00:00Z",
  };
}

const identities: ControlIdentity[] = [
  { account: account("self-id", "owner"), relationship: "self", label: "Self", selected: true },
  { account: account("linked-id", "partner", true), relationship: "linked", label: "Partner", selected: false },
];

describe("ControlIdentitySelector", () => {
  beforeEach(() => setLocaleForTest("en"));

  it("presents Self as the normal per-session context and exposes active links", async () => {
    const onOpenChange = vi.fn();
    const onSelect = vi.fn(async () => undefined);
    const { container } = render(
      <ControlIdentitySelector
        identities={identities}
        open
        restoreFocusOnClose
        onOpenChange={onOpenChange}
        onSelect={onSelect}
        onLogout={vi.fn(async () => undefined)}
      />,
    );

    expect(screen.getByRole("button", { name: /Control profile: Self/i })).toHaveAttribute("aria-expanded", "true");
    const choices = screen.getAllByRole("radio");
    expect(choices[0]).toHaveAttribute("aria-checked", "true");
    expect(choices[1]).toHaveAttribute("aria-checked", "false");
    expect(screen.getByText("partner · Linked account")).toBeInTheDocument();
    expect(screen.getByText(/does not sign in as another account or transfer device control/i)).toBeInTheDocument();
    const image = container.querySelector("img");
    expect(image).toHaveAttribute("src", expect.stringContaining("/api/accounts/linked-id/profile-image"));
    fireEvent.error(image!);
    expect(screen.getByText("P")).toBeInTheDocument();

    fireEvent.click(choices[1]);
    await waitFor(() => expect(onSelect).toHaveBeenCalledWith("linked-id"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("keeps the selector open and reports a rejected context change", async () => {
    const onOpenChange = vi.fn();
    render(
      <ControlIdentitySelector
        identities={identities}
        open
        restoreFocusOnClose
        onOpenChange={onOpenChange}
        onSelect={vi.fn(async () => { throw new Error("Link is no longer active."); })}
        onLogout={vi.fn(async () => undefined)}
      />,
    );

    fireEvent.click(screen.getAllByRole("radio")[1]);
    expect(await screen.findByRole("alert")).toHaveTextContent("Link is no longer active.");
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
