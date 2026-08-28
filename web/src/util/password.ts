export const MIN_PASSWORD_CHARACTERS = 8;

export type PasswordConfirmationState = "empty" | "match" | "mismatch";

// Array.from counts Unicode code points, matching Go's utf8.RuneCountInString.
// The backend remains authoritative and also enforces the 1024-byte upper bound.
export function passwordCharacterCount(value: string): number {
  return Array.from(value).length;
}

export function passwordMeetsMinimum(value: string): boolean {
  return passwordCharacterCount(value) >= MIN_PASSWORD_CHARACTERS;
}

export function passwordConfirmationState(
  password: string,
  confirmation: string,
): PasswordConfirmationState {
  if (!confirmation) return "empty";
  return password === confirmation ? "match" : "mismatch";
}
