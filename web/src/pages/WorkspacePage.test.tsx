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

  test("shows Chinese failure message from API", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-3",
          prompt: "海边",
          ratio: "1:1",
          size: "1024x1024",
          status: "failed",
          error_message: "生成失败：余额不足",
          created_at: "2026-04-30T08:00:00Z",
          completed_at: "2026-04-30T08:01:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "海边");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    expect(await screen.findByText("生成失败：余额不足")).toBeInTheDocument();
  });
});
