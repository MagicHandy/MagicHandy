import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { PasswordConfirmationField } from "./PasswordConfirmationField";

function Fixture() {
  const [confirmation, setConfirmation] = useState("");
  return (
    <PasswordConfirmationField
      password="eight888"
      confirmation={confirmation}
      onChange={setConfirmation}
    />
  );
}

describe("PasswordConfirmationField", () => {
  it("gives live visual and accessible mismatch then match feedback", () => {
    render(<Fixture />);
    const input = screen.getByLabelText("Confirm password");

    expect(input).toHaveAttribute("aria-invalid", "false");
    expect(screen.queryByText("Passwords match.")).not.toBeInTheDocument();

    fireEvent.change(input, { target: { value: "eight889" } });
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("The passwords do not match.")).toHaveAttribute("data-state", "mismatch");

    fireEvent.change(input, { target: { value: "eight888" } });
    expect(input).toHaveAttribute("aria-invalid", "false");
    expect(screen.getByText("Passwords match.")).toHaveAttribute("data-state", "match");
  });
});
