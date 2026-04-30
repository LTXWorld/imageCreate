import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, test, vi } from "vitest";

import type { User } from "../api/client";
import { WorkspacePage } from "./WorkspacePage";

const user: User = {
  id: "user-1",
  username: "alice",
  role: "user",
  status: "active",
  creditBalance: 8,
};

function jsonResponse(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

describe("WorkspacePage", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  test("shows simple point, refund, and retention guidance", () => {
    render(<WorkspacePage user={user} />);
    expect(screen.getByText("输入提示词，选择画面比例后开始生成。每次生成 1 张图，扣 1 点；失败会自动退回点数。生成图片保留 30 天。")).toBeInTheDocument();
  });

  test("creates a generation with prompt and ratio", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() =>
        jsonResponse({
          task: {
            id: "task-1",
            prompt: "一只杯子",
            ratio: "1:1",
            size: "1024x1024",
            status: "queued",
            created_at: "2026-04-30T08:00:00Z",
          },
        }),
      );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "一只杯子");
    await userEvent.click(screen.getByRole("button", { name: "1:1" }));
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/generations",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ prompt: "一只杯子", ratio: "1:1" }),
        }),
      );
    });
  });

  test("shows running state while polling", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockImplementationOnce(() =>
        jsonResponse({
          task: {
            id: "task-2",
            prompt: "山谷",
            ratio: "16:9",
            size: "1024x576",
            status: "queued",
            created_at: "2026-04-30T08:00:00Z",
          },
        }),
      )
      .mockImplementation(() =>
        jsonResponse({
          task: {
            id: "task-2",
            prompt: "山谷",
            ratio: "16:9",
            size: "1024x576",
            status: "running",
            created_at: "2026-04-30T08:00:00Z",
          },
        }),
      );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "山谷");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    expect((await screen.findAllByText("生成中")).length).toBeGreaterThan(0);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/generations/task-2",
        expect.objectContaining({ credentials: "include" }),
      );
    }, { timeout: 3000 });
  });

  test("shows fixed failure guidance without upstream details", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-3",
          prompt: "海边",
          ratio: "1:1",
          size: "1024x1024",
          status: "failed",
          error_message: "upstream internal details",
          created_at: "2026-04-30T08:00:00Z",
          completed_at: "2026-04-30T08:01:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "海边");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    expect(await screen.findByText("生成失败，已退回 1 点，可调整提示词后重试。")).toBeInTheDocument();
    expect(screen.queryByText("upstream internal details")).not.toBeInTheDocument();
  });

  test("shows sanitized failure guidance for known safe error codes", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-4",
          prompt: "海边",
          ratio: "1:1",
          size: "1024x1024",
          status: "failed",
          error_code: "content_rejected",
          message: "提示词可能包含不支持生成的内容，请调整描述后重试。",
          created_at: "2026-04-30T08:00:00Z",
          completed_at: "2026-04-30T08:01:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "海边");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    expect(await screen.findByText("生成失败，已退回 1 点，可调整提示词后重试。")).toBeInTheDocument();
    expect(screen.getByText("提示词可能包含不支持生成的内容，请调整描述后重试。")).toBeInTheDocument();
  });
});
