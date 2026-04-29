import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, test, vi } from "vitest";

import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  test("renders Chinese login form", () => {
    render(<LoginPage />);

    expect(screen.getByLabelText("用户名")).toBeInTheDocument();
    expect(screen.getByLabelText("密码")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "登录" })).toBeInTheDocument();
  });

  test("submits username and password to login API", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ id: "user-1", username: "alice" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );

    render(<LoginPage />);

    await userEvent.type(screen.getByLabelText("用户名"), "alice");
    await userEvent.type(screen.getByLabelText("密码"), "secret");
    await userEvent.click(screen.getByRole("button", { name: "登录" }));

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/login",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ username: "alice", password: "secret" }),
      }),
    );
  });
});
