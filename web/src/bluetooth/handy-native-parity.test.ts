import { describe, expect, it } from "vitest";
import vectors from "../../../internal/handyble/testdata/browser-wire-vectors.json";
import { encodeHandyRequest } from "./handy-ble-codec";

describe("shared native/browser wire fixtures", () => {
  for (const vector of vectors) it(`${vector.path} ${vector.id}`, () => {
    const encoded = encodeHandyRequest(vector.path, vector.body, vector.id);
    expect(Array.from(encoded, byte => byte.toString(16).padStart(2, "0")).join("")).toBe(vector.hex);
  });
});
