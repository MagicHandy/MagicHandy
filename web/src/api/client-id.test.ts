import { describe, expect, it, vi } from "vitest";
import { resolveControllerClientID } from "./client";

function memoryStorage(initial = "") {
  let value = initial;
  return {
    getItem: vi.fn(() => value || null),
    setItem: vi.fn((_key: string, next: string) => {
      value = next;
    }),
  };
}

describe("controller client identity", () => {
  it("reuses a tab identity across reloads", () => {
    const storage = memoryStorage("ui-existing");

    expect(resolveControllerClientID(storage, "reload", () => "ui-new")).toBe("ui-existing");
    expect(storage.setItem).not.toHaveBeenCalled();
  });

  it("replaces a copied identity in a newly opened or duplicated tab", () => {
    const storage = memoryStorage("ui-copied");

    expect(resolveControllerClientID(storage, "navigate", () => "ui-new")).toBe("ui-new");
    expect(storage.setItem).toHaveBeenCalledWith("magichandy-controller-tab-id", "ui-new");
  });

  it("mints an in-memory identity when browser storage is unavailable", () => {
    const blocked = {
      getItem: vi.fn(() => {
        throw new DOMException("blocked");
      }),
      setItem: vi.fn(() => {
        throw new DOMException("blocked");
      }),
    };

    expect(resolveControllerClientID(blocked, "navigate", () => "ui-memory")).toBe("ui-memory");
  });
});
