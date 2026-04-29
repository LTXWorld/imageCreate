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
