import { render, screen, waitFor, within } from "@testing-library/react";
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

  test("updates displayed current-user credits after admin adjusts own credits", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/auth/me") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              user: {
                id: "admin-1",
                username: "admin",
                role: "admin",
                status: "active",
                credit_balance: 99,
                daily_free_credit_limit: 5,
                daily_free_credit_balance: 5,
                paid_credit_balance: 94,
              },
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      if (url === "/api/admin/users" && !init?.method) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              users: [
                {
                  id: "admin-1",
                  username: "admin",
                  role: "admin",
                  status: "active",
                  credit_balance: 99,
                  daily_free_credit_limit: 5,
                  daily_free_credit_balance: 5,
                  paid_credit_balance: 94,
                  created_at: "2026-04-30T08:00:00Z",
                  updated_at: "2026-04-30T08:00:00Z",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      if (url === "/api/admin/invites" && !init?.method) {
        return Promise.resolve(new Response(JSON.stringify({ invites: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }));
      }
      if (url === "/api/admin/audit-logs" && !init?.method) {
        return Promise.resolve(new Response(JSON.stringify({ audit_logs: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }));
      }
      if (url === "/api/admin/generation-tasks" && !init?.method) {
        return Promise.resolve(new Response(JSON.stringify({ tasks: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }));
      }
      if (url === "/api/admin/users/admin-1/credits" && init?.method === "POST") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              user: {
                id: "admin-1",
                username: "admin",
                role: "admin",
                status: "active",
                credit_balance: 102,
                daily_free_credit_limit: 5,
                daily_free_credit_balance: 5,
                paid_credit_balance: 97,
                created_at: "2026-04-30T08:00:00Z",
                updated_at: "2026-04-30T09:00:00Z",
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
      expect(screen.getAllByText("99 点").length).toBeGreaterThan(0);
    });

    await userEvent.click(await screen.findByRole("tab", { name: "额度" }));
    const row = await screen.findByRole("row", { name: /admin/ });
    await userEvent.type(within(row).getByLabelText("调整 admin 的积分"), "3");
    await userEvent.type(within(row).getByLabelText("调整 admin 的原因"), "自测补额");
    await userEvent.click(within(row).getByRole("button", { name: "提交调整" }));

    await waitFor(() => {
      expect(screen.getAllByText("102 点").length).toBeGreaterThan(0);
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/users/admin-1/credits",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ amount: 3, reason: "自测补额" }),
      }),
    );
  });
});
