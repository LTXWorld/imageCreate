# Image Click Preview Design

## Goal

用户生成图片成功后，可以在创作台和历史记录中点击图片放大预览。现有下载按钮继续保留，图片读取仍使用当前的用户鉴权图片接口，不新增后端接口。

## Scope

- 创作台当前任务成功图片支持点击预览。
- 历史记录成功图片缩略图支持点击预览。
- 预览使用全屏遮罩弹窗，图片按屏幕自适应完整显示，不裁切。
- 用户可以通过关闭按钮、点击遮罩、按 `Escape` 关闭预览。
- 管理员审计页继续不展示图片 URL 或图片预览。

## Approach

新增一个前端共享组件 `ImagePreviewDialog`，接收当前预览图片的 `src`、`alt` 和关闭回调。`WorkspacePage` 与 `HistoryPage` 各自维护当前预览图片状态；成功图片从普通 `img` 包装为可点击按钮，点击时打开弹窗。

弹窗使用原生 React 条件渲染，不引入新的 UI 依赖。打开时渲染 `role="dialog"` 和 `aria-modal="true"`，关闭按钮有清晰的中文可访问名称。组件监听 `keydown`，仅在打开时响应 `Escape`。

## Data Flow

现有 `GenerationTask.imageUrl` 继续来自 `normalizeGenerationTask`。页面将成功任务的 `imageUrl` 传给预览状态，再传给 `ImagePreviewDialog`。弹窗只负责展示同一个 URL，不请求额外数据，也不改变下载文件名逻辑。

## Styling

图片按钮去掉默认按钮外观，保留当前缩略图尺寸和边框。鼠标悬停时显示可点击的轻微强调；键盘聚焦时显示清晰 outline。弹窗遮罩使用半透明深色背景，图片最大宽高限制在视口内，避免遮挡关闭按钮。

## Testing

- `WorkspacePage` 测试：生成成功后点击结果图片，显示预览弹窗；点击关闭按钮或按 `Escape` 后弹窗消失。
- `HistoryPage` 测试：历史成功图片可点击打开预览，下载链接仍使用原文件名。
- 保持现有管理员审计测试不变，确认审计页不显示图片链接或预览。

## Non-Goals

- 不做图片缩放倍率、拖拽平移或轮播。
- 不改变图片存储、后端鉴权或下载接口。
- 不在管理员页面提供用户图片预览。
