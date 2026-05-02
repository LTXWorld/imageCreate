import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

  test("refreshes displayed credits after creating a generation", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input);
      if (url === "/api/auth/me" && fetchMock.mock.calls.length === 1) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              user: {
                id: "user-1",
                username: "alice",
                role: "user",
                status: "active",
                credit_balance: 8,
                daily_free_credit_limit: 5,
                daily_free_credit_balance: 5,
                paid_credit_balance: 3,
              },
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      if (url === "/api/generations") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              task: {
                id: "task-1",
                prompt: "一只杯子",
                ratio: "1:1",
                size: "1024x1024",
                status: "queued",
                created_at: "2026-04-30T08:00:00Z",
              },
            }),
            {
              status: 201,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      if (url === "/api/auth/me") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              user: {
                id: "user-1",
                username: "alice",
                role: "user",
                status: "active",
                credit_balance: 7,
                daily_free_credit_limit: 5,
                daily_free_credit_balance: 4,
                paid_credit_balance: 3,
              },
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      return Promise.reject(new Error(`unexpected request: ${url}`));
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getAllByText("8 点").length).toBeGreaterThan(0);
    });

    await userEvent.type(screen.getByLabelText("提示词"), "一只杯子");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    await waitFor(() => {
      expect(screen.getAllByText("7 点").length).toBeGreaterThan(0);
    });
    expect(screen.getByText("今日免费额度 4/5")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/me",
      expect.objectContaining({ credentials: "include" }),
    );
  });
});
