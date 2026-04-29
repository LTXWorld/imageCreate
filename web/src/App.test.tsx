import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, test, vi } from "vitest";

import { App } from "./App";

describe("App", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  test("restores an existing session from auth me on startup", async () => {
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
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      );

    render(<App />);

    expect(screen.getByText("正在确认登录状态...")).toBeInTheDocument();
    expect(screen.queryByText("未登录")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "登录" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "注册" })).not.toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("alice")).toBeInTheDocument();
    });
    expect(screen.getByText("图像生成")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/me",
      expect.objectContaining({ credentials: "include" }),
    );
  });
});
