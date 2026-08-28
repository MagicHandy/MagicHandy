import { describe, expect, it } from "vitest";
import {
  MIN_PASSWORD_CHARACTERS,
  passwordCharacterCount,
  passwordConfirmationState,
  passwordMeetsMinimum,
} from "./password";

describe("password policy helpers", () => {
  it("uses an eight-character floor for ASCII and multibyte Unicode", () => {
    expect(MIN_PASSWORD_CHARACTERS).toBe(8);
    expect(passwordMeetsMinimum("1234567")).toBe(false);
    expect(passwordMeetsMinimum("12345678")).toBe(true);
    expect(passwordCharacterCount("🔒🔒🔒🔒🔒🔒🔒🔒")).toBe(8);
    expect(passwordMeetsMinimum("🔒🔒🔒🔒🔒🔒🔒🔒")).toBe(true);
  });

  it("reports confirmation only after the user starts typing it", () => {
    expect(passwordConfirmationState("password", "")).toBe("empty");
    expect(passwordConfirmationState("password", "passw0rd")).toBe("mismatch");
    expect(passwordConfirmationState("password", "password")).toBe("match");
  });
});
