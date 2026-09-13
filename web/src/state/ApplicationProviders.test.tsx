import { useState } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { AppState } from "../api/types";
import { ApplicationProviders } from "./ApplicationProviders";

const auth = vi.hoisted(() => ({ status: { authentication_required: true, authenticated: true, session_id: "admin-login", ui_locale: "en" } }));
vi.mock("./auth", () => ({ useAuth: () => auth }));
vi.mock("../api/client", () => ({
  clientId: "audience-regression",
  COMMAND_RECOVERED_EVENT: "magichandy:command-recovered",
  api: { getState: vi.fn() },
}));
vi.mock("../App", async () => {
  const { useAppState, useToast, useNotifications } = await import("./app-state");
  return { App: function AudienceFixture() {
    const { state, refresh } = useAppState();
    const { show } = useToast();
    const { items } = useNotifications();
    const [draft, setDraft] = useState("");
    return <>
      <output>{state?.data_dir || "No private host path"}</output>
      <input aria-label="Settings draft" value={draft} onChange={(event) => setDraft(event.target.value)} />
      <button onClick={() => void refresh()}>Refresh fixture</button>
      <button onClick={() => show("private administrator notification")}>Notify fixture</button>
      <output>{items.map((item) => item.title).join(", ")}</output>
    </>;
  } };
});

class EventSourceFixture {
  static sources: EventSourceFixture[] = [];
  close = vi.fn();
  constructor() { EventSourceFixture.sources.push(this); }
  addEventListener() {}
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("discards host snapshots, drafts and delayed responses when the login changes", async () => {
  vi.stubGlobal("EventSource", EventSourceFixture);
  const snapshot = (dataDir: string) => ({ data_dir: dataDir, controller: { read_only: true }, motion: { available: true } }) as AppState;
  let completeOldRead!: (value: AppState) => void;
  let completeObserverRead!: (value: AppState) => void;
  vi.mocked(api.getState)
    .mockResolvedValueOnce(snapshot("private administrator path"))
    .mockImplementationOnce(() => new Promise((resolve) => { completeOldRead = resolve; }))
    .mockImplementationOnce(() => new Promise((resolve) => { completeObserverRead = resolve; }));
  const view = render(<ApplicationProviders />);
  expect(await screen.findByText("private administrator path")).toBeInTheDocument();
  fireEvent.change(screen.getByRole("textbox", { name: "Settings draft" }), { target: { value: "private worker arguments" } });
  fireEvent.click(screen.getByRole("button", { name: "Refresh fixture" }));
  fireEvent.click(screen.getByRole("button", { name: "Notify fixture" }));
  expect(screen.getAllByText("private administrator notification").length).toBeGreaterThan(0);
  const oldStream = EventSourceFixture.sources[0];
  auth.status = { ...auth.status, session_id: "observer-login" };
  view.rerender(<ApplicationProviders />);
  expect(screen.queryByText("private administrator path")).not.toBeInTheDocument();
  expect(screen.queryByText("private administrator notification")).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "Settings draft" })).toHaveValue("");
  expect(oldStream.close).toHaveBeenCalled();
  await act(async () => { completeOldRead(snapshot("late private administrator path")); });
  expect(screen.queryByText("late private administrator path")).not.toBeInTheDocument();
  await act(async () => { completeObserverRead(snapshot("")); });
  expect(screen.getByText("No private host path")).toBeInTheDocument();
  expect(window.sessionStorage.getItem("magichandy-notifications-v1")).not.toContain("private administrator notification");
});
