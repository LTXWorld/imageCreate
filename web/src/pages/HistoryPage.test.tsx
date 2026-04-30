import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, test, vi } from "vitest";

import { HistoryPage } from "./HistoryPage";

function jsonResponse(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

describe("HistoryPage", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  test("renders only tasks returned by the API response", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      await jsonResponse({
        tasks: [
          {
            id: "task-1",
            prompt: "我的山谷",
            ratio: "16:9",
            size: "1024x576",
            status: "succeeded",
            image_url: "/api/generations/task-1/image",
            created_at: "2026-04-30T08:00:00Z",
            completed_at: "2026-04-30T08:01:00Z",
          },
          {
            id: "task-2",
            prompt: "我的港口",
            ratio: "4:3",
            size: "1024x768",
            status: "failed",
            error_message: "生成失败",
            created_at: "2026-04-30T09:00:00Z",
          },
        ],
      }),
    );

    render(<HistoryPage />);

    expect(screen.getByText("这里只显示最近 30 天的生成记录。请及时下载需要长期保存的图片。")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("我的山谷")).toBeInTheDocument();
    });

    expect(screen.getByText("我的港口")).toBeInTheDocument();
    const downloadLink = screen.getByRole("link", { name: "下载图片" });
    expect(downloadLink).toHaveAttribute("href", "/api/generations/task-1/image");
    expect(downloadLink).toHaveAttribute("download", "imagecreate-task-1-16-9.png");
    // The frontend can only render tasks returned by the user-scoped API.
    expect(screen.queryByText("其他用户的图片")).not.toBeInTheDocument();
  });
});
