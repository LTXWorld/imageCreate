import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, test, vi } from "vitest";

import type { User } from "../api/client";
import { AdminPage } from "./AdminPage";

const adminUser: User = {
  id: "admin-1",
  username: "admin",
  role: "admin",
  status: "active",
  creditBalance: 99,
};

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function mockAdminFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
    const path = String(input);
    if (path === "/api/admin/users" && !init?.method) {
      return jsonResponse({
        users: [
          {
            id: "user-1",
            username: "alice",
            role: "user",
            status: "active",
            credit_balance: 8,
            created_at: "2026-04-30T08:00:00Z",
            updated_at: "2026-04-30T08:00:00Z",
          },
        ],
      });
    }
    if (path === "/api/admin/invites" && !init?.method) {
      return jsonResponse({
        invites: [
          {
            id: "invite-1",
            code: "invite-demo",
            initial_credits: 5,
            status: "unused",
            created_at: "2026-04-30T08:00:00Z",
          },
        ],
      });
    }
    if (path === "/api/admin/audit-logs" && !init?.method) {
      return jsonResponse({ audit_logs: [] });
    }
    if (path === "/api/admin/generation-tasks" && !init?.method) {
      return jsonResponse({
        tasks: [
          {
            id: "task-1",
            user_id: "user-1",
            username: "alice",
            prompt: "审计里的山谷",
            size: "1024x1024",
            status: "succeeded",
            latency_ms: 1240,
            image_url: "/api/generations/task-1/image",
            created_at: "2026-04-30T08:00:00Z",
            completed_at: "2026-04-30T08:01:00Z",
          },
        ],
      });
    }
    if (path === "/api/admin/invites" && init?.method === "POST") {
      return jsonResponse({
        invite: {
          id: "invite-2",
          code: "invite-new",
          initial_credits: 12,
          status: "unused",
          created_at: "2026-04-30T09:00:00Z",
        },
      }, 201);
    }
    if (path === "/api/admin/users/user-1/credits" && init?.method === "POST") {
      return jsonResponse({
        user: {
          id: "user-1",
          username: "alice",
          role: "user",
          status: "active",
          credit_balance: 11,
          created_at: "2026-04-30T08:00:00Z",
          updated_at: "2026-04-30T09:00:00Z",
        },
      });
    }
    if (path === "/api/admin/password" && init?.method === "POST") {
      return jsonResponse({ ok: true });
    }
    if (path === "/api/admin/users/user-1/password" && init?.method === "POST") {
      return jsonResponse({ ok: true });
    }
    return jsonResponse({ message: `unexpected request: ${path}` }, 500);
  });
}

describe("AdminPage", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  test("shows user management for admins", async () => {
    mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    expect(await screen.findByText("用户管理")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("8 点")).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
  });

  test("creates an invite with initial credits", async () => {
    const fetchMock = mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    await userEvent.click(await screen.findByRole("tab", { name: "邀请码" }));
    await userEvent.clear(screen.getByLabelText("初始额度"));
    await userEvent.type(screen.getByLabelText("初始额度"), "12");
    await userEvent.click(screen.getByRole("button", { name: "创建邀请码" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/admin/invites",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ code: "", initial_credits: 12 }),
        }),
      );
    });
  });

  test("adjusts user credits", async () => {
    const fetchMock = mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    await userEvent.click(await screen.findByRole("tab", { name: "额度" }));
    const row = await screen.findByRole("row", { name: /alice/ });
    await userEvent.type(within(row).getByLabelText("调整 alice 的积分"), "3");
    await userEvent.type(within(row).getByLabelText("调整 alice 的原因"), "活动补偿");
    await userEvent.click(within(row).getByRole("button", { name: "提交调整" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/admin/users/user-1/credits",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ amount: 3, reason: "活动补偿" }),
        }),
      );
    });
  });

  test("changes the current admin password from the security tab", async () => {
    const fetchMock = mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    await userEvent.click(await screen.findByRole("tab", { name: "安全" }));
    const currentPasswordInput = screen.getByLabelText("当前密码");
    const newPasswordInput = screen.getByLabelText("新密码");
    const confirmPasswordInput = screen.getByLabelText("确认新密码");
    expect(currentPasswordInput).toHaveAttribute("name", "current-password");
    expect(currentPasswordInput).toHaveAttribute("autocomplete", "current-password");
    expect(newPasswordInput).toHaveAttribute("name", "new-password");
    expect(newPasswordInput).toHaveAttribute("autocomplete", "new-password");
    expect(confirmPasswordInput).toHaveAttribute("name", "confirm-password");
    expect(confirmPasswordInput).toHaveAttribute("autocomplete", "new-password");
    await userEvent.type(currentPasswordInput, "old-password");
    await userEvent.type(newPasswordInput, "new-password");
    await userEvent.type(confirmPasswordInput, "new-password");
    await userEvent.click(screen.getByRole("button", { name: "更新密码" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/admin/password",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ current_password: "old-password", new_password: "new-password" }),
        }),
      );
    });
    expect(await screen.findByText("密码已更新")).toBeInTheDocument();
  });

  test("clears success notices when switching tabs", async () => {
    mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    await userEvent.click(await screen.findByRole("tab", { name: "安全" }));
    await userEvent.type(screen.getByLabelText("当前密码"), "old-password");
    await userEvent.type(screen.getByLabelText("新密码"), "new-password");
    await userEvent.type(screen.getByLabelText("确认新密码"), "new-password");
    await userEvent.click(screen.getByRole("button", { name: "更新密码" }));

    expect(await screen.findByText("密码已更新")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "用户" }));

    expect(screen.queryByText("密码已更新")).not.toBeInTheDocument();
  });

  test("resets a user password from the users table", async () => {
    const fetchMock = mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    const row = await screen.findByRole("row", { name: /alice/ });
    await userEvent.click(within(row).getByRole("button", { name: "重置密码" }));
    const resetPasswordInput = screen.getByLabelText("alice 的新密码");
    expect(resetPasswordInput).toHaveAttribute("name", "reset-password");
    expect(resetPasswordInput).toHaveAttribute("autocomplete", "new-password");
    await userEvent.type(resetPasswordInput, "new-password");
    await userEvent.click(screen.getByRole("button", { name: "确认重置" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/admin/users/user-1/password",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ new_password: "new-password" }),
        }),
      );
    });
    expect(await screen.findByText("用户密码已重置")).toBeInTheDocument();
  });

  test("shows the simple credit rule in the credit tab", async () => {
    mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    await userEvent.click(await screen.findByRole("tab", { name: "额度" }));

    expect(screen.getByText("点数规则：每次生成扣 1 点，失败自动退回 1 点。")).toBeInTheDocument();
  });

  test("does not render image links in audit task table", async () => {
    mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    await userEvent.click(await screen.findByRole("tab", { name: "审计" }));

    expect(await screen.findByText("审计里的山谷")).toBeInTheDocument();
    expect(screen.getByText("succeeded")).toBeInTheDocument();
    expect(screen.getByText("1024x1024")).toBeInTheDocument();
    expect(screen.getByText("1240 ms")).toBeInTheDocument();
    expect(screen.queryByText("/api/generations/task-1/image")).not.toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });
});
