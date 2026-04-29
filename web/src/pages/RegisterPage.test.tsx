import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, test, vi } from "vitest";

import { RegisterPage } from "./RegisterPage";

describe("RegisterPage", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  test("renders invite code on register page", () => {
    render(<RegisterPage />);

    expect(screen.getByLabelText("邀请码")).toBeInTheDocument();
  });

  test("submits invite code to register API", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(
          JSON.stringify({
            user: {
              id: "user-1",
              username: "alice",
              role: "user",
              status: "active",
              credit_balance: 5,
            },
          }),
          {
            status: 201,
            headers: { "Content-Type": "application/json" },
          },
        ),
      );

    render(<RegisterPage />);

    await userEvent.type(screen.getByLabelText("用户名"), "alice");
    await userEvent.type(screen.getByLabelText("密码"), "secret");
    await userEvent.type(screen.getByLabelText("邀请码"), "invite-1");
    await userEvent.click(screen.getByRole("button", { name: "注册" }));

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/register",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          username: "alice",
          password: "secret",
          invite_code: "invite-1",
        }),
      }),
    );
  });
});
