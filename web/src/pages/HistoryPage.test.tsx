import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
            is_favorite: true,
            title: "山谷作品",
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

    expect(screen.getByText("记录将保留 30 天，请及时下载需要长期保存的图片。")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("我的山谷")).toBeInTheDocument();
    });

    expect(screen.getByText("我的港口")).toBeInTheDocument();
    expect(screen.getByText("山谷作品")).toBeInTheDocument();
    expect(screen.getByLabelText("已收藏")).toBeInTheDocument();
    const downloadLink = screen.getByRole("link", { name: "下载图片" });
    expect(downloadLink).toHaveAttribute("href", "/api/generations/task-1/image");
    expect(downloadLink).toHaveAttribute("download", "imagecreate-task-1-16-9.png");
    await userEvent.click(screen.getByRole("button", { name: "查看作品详情：我的山谷" }));

    const dialog = screen.getByRole("dialog", { name: "作品详情" });
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getByRole("img", { name: "我的山谷" })).toHaveAttribute(
      "src",
      "/api/generations/task-1/image",
    );
    expect(within(dialog).getByText("提示词")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "关闭作品详情" }));
    expect(screen.queryByRole("dialog", { name: "作品详情" })).not.toBeInTheDocument();
    // The frontend can only render tasks returned by the user-scoped API.
    expect(screen.queryByText("其他用户的图片")).not.toBeInTheDocument();
  });

  test("supports favorite, title editing, and prompt reuse", async () => {
    const onReusePrompt = vi.fn();
    const fetchMock = vi.spyOn(globalThis, "fetch");
    fetchMock
      .mockResolvedValueOnce(await jsonResponse({
        tasks: [
          {
            id: "task-1",
            prompt: "我的山谷",
            ratio: "16:9",
            size: "1024x576",
            status: "succeeded",
            image_url: "/api/generations/task-1/image",
            created_at: "2026-04-30T08:00:00Z",
            is_favorite: false,
          },
        ],
      }))
      .mockResolvedValueOnce(await jsonResponse({
        task: {
          id: "task-1",
          prompt: "我的山谷",
          ratio: "16:9",
          size: "1024x576",
          status: "succeeded",
          image_url: "/api/generations/task-1/image",
          created_at: "2026-04-30T08:00:00Z",
          is_favorite: true,
        },
      }))
      .mockResolvedValueOnce(await jsonResponse({
        task: {
          id: "task-1",
          prompt: "我的山谷",
          ratio: "16:9",
          size: "1024x576",
          status: "succeeded",
          image_url: "/api/generations/task-1/image",
          created_at: "2026-04-30T08:00:00Z",
          is_favorite: true,
          title: "新标题",
        },
      }));

    render(<HistoryPage onReusePrompt={onReusePrompt} />);

    await waitFor(() => expect(screen.getByText("我的山谷")).toBeInTheDocument());
    await userEvent.click(screen.getByRole("button", { name: "收藏作品" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "取消收藏" })).toBeInTheDocument());

    await userEvent.click(screen.getByRole("button", { name: "编辑标题" }));
    await userEvent.type(screen.getByLabelText("作品标题"), "新标题");
    await userEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() => expect(screen.getByText("新标题")).toBeInTheDocument());

    await userEvent.click(screen.getByRole("button", { name: "复制提示词再生成" }));
    expect(onReusePrompt).toHaveBeenCalledWith(expect.objectContaining({ prompt: "我的山谷", ratio: "16:9" }));
  });

  test("filters history and supports image reuse", async () => {
    const onReuseImage = vi.fn();
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      await jsonResponse({
        tasks: [
          {
            id: "task-1",
            prompt: "收藏作品",
            ratio: "1:1",
            size: "1024x1024",
            status: "succeeded",
            image_url: "/api/generations/task-1/image",
            created_at: "2026-04-30T08:00:00Z",
            is_favorite: true,
          },
          {
            id: "task-2",
            prompt: "失败作品",
            ratio: "1:1",
            size: "1024x1024",
            status: "failed",
            created_at: "2026-04-30T09:00:00Z",
          },
        ],
      }),
    );

    render(<HistoryPage onReuseImage={onReuseImage} />);

    await waitFor(() => expect(screen.getByText("收藏作品")).toBeInTheDocument());
    expect(screen.getByText("失败作品")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "已收藏" }));
    expect(screen.getByText("收藏作品")).toBeInTheDocument();
    expect(screen.queryByText("失败作品")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "作为参考图再创作" }));
    expect(onReuseImage).toHaveBeenCalledWith(expect.objectContaining({ id: "task-1", imageUrl: "/api/generations/task-1/image" }));
  });
});
