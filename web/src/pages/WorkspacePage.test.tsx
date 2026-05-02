import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  dailyFreeCreditLimit: 5,
  dailyFreeCreditBalance: 5,
  paidCreditBalance: 3,
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

  test("shows cancellation window and retention guidance", () => {
    render(<WorkspacePage user={user} />);
    expect(screen.getByText("输入提示词，选择画面比例后开始生成。提交后短时间内可取消；开始生成后无法取消消耗。生成图片保留 30 天。")).toBeInTheDocument();
  });

  test("shows split credit balances", () => {
    render(<WorkspacePage user={user} />);

    expect(screen.getByText("当前余额")).toBeInTheDocument();
    expect(screen.getByText("8 点")).toBeInTheDocument();
    expect(screen.getByText("今日免费额度 5/5")).toBeInTheDocument();
    expect(screen.getByText("付费额度 3")).toBeInTheDocument();
  });

  test("shows private support contact guidance", () => {
    render(<WorkspacePage user={user} />);

    expect(screen.getByRole("region", { name: "专属服务" })).toBeInTheDocument();
    expect(screen.getByText("加 QQ 或微信获取帮助")).toBeInTheDocument();
    expect(screen.getByText("QQ")).toBeInTheDocument();
    expect(screen.getByText("微信")).toBeInTheDocument();
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

  test("submits prompt value that was filled without a React change event", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() =>
        jsonResponse({
          task: {
            id: "task-dom-prompt",
            prompt: "浏览器填充的提示词",
            ratio: "1:1",
            size: "1024x1024",
            status: "queued",
            created_at: "2026-04-30T08:00:00Z",
          },
        }),
      );

    render(<WorkspacePage user={user} />);

    const promptInput = screen.getByLabelText("提示词") as HTMLTextAreaElement;
    promptInput.value = "浏览器填充的提示词";
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/generations",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ prompt: "浏览器填充的提示词", ratio: "1:1" }),
        }),
      );
    });
  });

  test("refreshes user credits after creating a generation", async () => {
    const onUserRefresh = vi.fn();
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-refresh-create",
          prompt: "一只杯子",
          ratio: "1:1",
          size: "1024x1024",
          status: "queued",
          created_at: "2026-04-30T08:00:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} onUserRefresh={onUserRefresh} />);

    await userEvent.type(screen.getByLabelText("提示词"), "一只杯子");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    await waitFor(() => {
      expect(onUserRefresh).toHaveBeenCalledTimes(1);
    });
  });

  test("polls active tasks every five seconds", async () => {
    vi.useFakeTimers();
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

    fireEvent.change(screen.getByLabelText("提示词"), { target: { value: "山谷" } });
    fireEvent.click(screen.getByRole("button", { name: "生成" }));
    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.getAllByText("生成中").length).toBeGreaterThan(0);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(4999);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(1);
      await Promise.resolve();
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/generations/task-2",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  test("refreshes user credits when polling reaches a final task status", async () => {
    vi.useFakeTimers();
    const onUserRefresh = vi.fn();
    vi.spyOn(globalThis, "fetch")
      .mockImplementationOnce(() =>
        jsonResponse({
          task: {
            id: "task-refresh-final",
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
            id: "task-refresh-final",
            prompt: "山谷",
            ratio: "16:9",
            size: "1024x576",
            status: "failed",
            error_code: "timeout",
            message: "生成超时，本次额度已退回，请稍后重试。",
            created_at: "2026-04-30T08:00:00Z",
            completed_at: "2026-04-30T08:01:00Z",
          },
        }),
      );

    render(<WorkspacePage user={user} onUserRefresh={onUserRefresh} />);

    fireEvent.change(screen.getByLabelText("提示词"), { target: { value: "山谷" } });
    fireEvent.click(screen.getByRole("button", { name: "生成" }));
    await act(async () => {
      await Promise.resolve();
    });

    expect(onUserRefresh).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(5000);
      await Promise.resolve();
    });

    expect(onUserRefresh).toHaveBeenCalledTimes(2);
  });

  test("cancels a queued generation and refreshes user credits", async () => {
    const onUserRefresh = vi.fn();
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockImplementationOnce(() =>
        jsonResponse({
          task: {
            id: "task-cancel",
            prompt: "写错的提示词",
            ratio: "1:1",
            size: "1024x1024",
            status: "queued",
            created_at: "2026-04-30T08:00:00Z",
          },
        }),
      )
      .mockImplementationOnce(() =>
        jsonResponse({
          task: {
            id: "task-cancel",
            prompt: "写错的提示词",
            ratio: "1:1",
            size: "1024x1024",
            status: "canceled",
            created_at: "2026-04-30T08:00:00Z",
            completed_at: "2026-04-30T08:00:10Z",
          },
        }),
      );

    render(<WorkspacePage user={user} onUserRefresh={onUserRefresh} />);

    await userEvent.type(screen.getByLabelText("提示词"), "写错的提示词");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));
    await userEvent.click(await screen.findByRole("button", { name: "取消本次提交" }));

    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/generations/task-cancel/cancel",
      expect.objectContaining({ method: "POST" }),
    );
    expect(await screen.findByText("已取消，本次额度已退回，可修改提示词后重新生成。")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "生成" })).toBeEnabled();
    expect(onUserRefresh).toHaveBeenCalledTimes(2);
  });

  test("does not offer cancellation after generation starts upstream", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-running-no-cancel",
          prompt: "已经开始的提示词",
          ratio: "1:1",
          size: "1024x1024",
          status: "running",
          created_at: "2026-04-30T08:00:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "已经开始的提示词");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    expect(await screen.findByText("已开始生成，请等待结果。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "取消本次提交" })).not.toBeInTheDocument();
  });

  test("shows a progress bar while a queued task is waiting", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-04-30T08:00:45Z"));
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-progress-queued",
          prompt: "森林小屋",
          ratio: "1:1",
          size: "1024x1024",
          status: "queued",
          created_at: "2026-04-30T08:00:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    fireEvent.change(screen.getByLabelText("提示词"), { target: { value: "森林小屋" } });
    fireEvent.click(screen.getByRole("button", { name: "生成" }));
    await act(async () => {
      await Promise.resolve();
    });

    const progress = screen.getByRole("progressbar", { name: "生成进度" });
    expect(progress).toHaveAttribute("aria-valuenow", "15");
    expect(screen.getByText("正在排队")).toBeInTheDocument();
    expect(screen.getByText("15%")).toBeInTheDocument();
  });

  test("advances local progress for a running task between polling requests", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-04-30T08:00:30Z"));
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-progress-running",
          prompt: "海上灯塔",
          ratio: "1:1",
          size: "1024x1024",
          status: "running",
          created_at: "2026-04-30T08:00:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    fireEvent.change(screen.getByLabelText("提示词"), { target: { value: "海上灯塔" } });
    fireEvent.click(screen.getByRole("button", { name: "生成" }));
    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.getByRole("progressbar", { name: "生成进度" })).toHaveAttribute("aria-valuenow", "36");
    expect(screen.getByText("正在绘制细节")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(screen.getByRole("progressbar", { name: "生成进度" })).toHaveAttribute("aria-valuenow", "37");
    expect(fetchMock).toHaveBeenCalledTimes(1);
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

  test("keeps failure guidance visible with stable progress", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-04-30T08:01:00Z"));
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-progress-failed",
          prompt: "海边",
          ratio: "1:1",
          size: "1024x1024",
          status: "failed",
          error_code: "timeout",
          message: "生成超时，本次额度已退回，请稍后重试。",
          created_at: "2026-04-30T08:00:00Z",
          completed_at: "2026-04-30T08:01:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    fireEvent.change(screen.getByLabelText("提示词"), { target: { value: "海边" } });
    fireEvent.click(screen.getByRole("button", { name: "生成" }));
    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.getByRole("progressbar", { name: "生成进度" })).toHaveAttribute("aria-valuenow", "47");
    expect(screen.getByText("生成未完成")).toBeInTheDocument();
    expect(screen.getByText("生成失败，已退回 1 点，可调整提示词后重试。")).toBeInTheDocument();
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

  test("shows a download link after generation succeeds", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-5",
          prompt: "星空城堡",
          ratio: "16:9",
          size: "1280x720",
          status: "succeeded",
          image_url: "/api/generations/task-5/image",
          created_at: "2026-04-30T08:00:00Z",
          completed_at: "2026-04-30T08:01:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "星空城堡");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    const downloadLink = await screen.findByRole("link", { name: "下载图片" });
    expect(downloadLink).toHaveAttribute("href", "/api/generations/task-5/image");
    expect(downloadLink).toHaveAttribute("download", "imagecreate-task-5-16-9.png");
  });

  test("shows completed progress when generation succeeds", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-progress-succeeded",
          prompt: "星空城堡",
          ratio: "16:9",
          size: "1280x720",
          status: "succeeded",
          image_url: "/api/generations/task-progress-succeeded/image",
          created_at: "2026-04-30T08:00:00Z",
          completed_at: "2026-04-30T08:01:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "星空城堡");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    const progress = await screen.findByRole("progressbar", { name: "生成进度" });
    expect(progress).toHaveAttribute("aria-valuenow", "100");
    expect(screen.getByText("生成完成")).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "下载图片" })).toBeInTheDocument();
  });
});
