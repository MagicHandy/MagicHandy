import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { SetupJob } from "../api/types";
import { useParakeetRepair } from "./useParakeetRepair";

vi.mock("../api/client", () => ({
  api: { setupStatus: vi.fn(), installSetupParakeet: vi.fn(), cancelSetupInstall: vi.fn() },
}));

const setupStatus = vi.mocked(api.setupStatus);
const installSetupParakeet = vi.mocked(api.installSetupParakeet);

const job = (status: SetupJob["status"]): SetupJob => ({
  id: "repair-1", kind: "parakeet", module: "parakeet", device: "cpu", status, message: "Parakeet model: 1 MiB of 100 MiB.", started_at: "now", updated_at: "now",
});

const settle = () => act(async () => {
  for (let step = 0; step < 6; step += 1) await Promise.resolve();
});

describe("useParakeetRepair", () => {
  beforeEach(() => {
    setupStatus.mockReset();
    installSetupParakeet.mockReset();
  });

  it("keeps the started repair when an older status read answers afterwards", async () => {
    let answerOldRead!: (value: Awaited<ReturnType<typeof api.setupStatus>>) => void;
    setupStatus.mockReturnValueOnce(new Promise((resolve) => { answerOldRead = resolve; }));
    installSetupParakeet.mockResolvedValue({ installation: job("running") });
    const { result, unmount } = renderHook(() => useParakeetRepair(true, vi.fn()));

    await act(async () => { await result.current.repair(); });
    expect(result.current.job?.status).toBe("running");
    expect(result.current.setupBusy).toBe(true);

    answerOldRead({ installation: undefined } as Awaited<ReturnType<typeof api.setupStatus>>);
    await settle();
    expect(result.current.job?.status).toBe("running");
    expect(result.current.setupBusy).toBe(true);
    unmount();
  });

  it("reports completion once per job", async () => {
    const onComplete = vi.fn();
    setupStatus.mockResolvedValue({ installation: job("complete") } as Awaited<ReturnType<typeof api.setupStatus>>);
    const { rerender } = renderHook(({ enabled }) => useParakeetRepair(enabled, onComplete), { initialProps: { enabled: true } });
    await settle();
    rerender({ enabled: false });
    rerender({ enabled: true });
    await settle();
    expect(onComplete).toHaveBeenCalledOnce();
  });
});
