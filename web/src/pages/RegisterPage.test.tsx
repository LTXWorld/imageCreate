import { render, screen } from "@testing-library/react";
import { describe, expect, test } from "vitest";

import { RegisterPage } from "./RegisterPage";

describe("RegisterPage", () => {
  test("renders invite code on register page", () => {
    render(<RegisterPage />);

    expect(screen.getByLabelText("邀请码")).toBeInTheDocument();
  });
});
