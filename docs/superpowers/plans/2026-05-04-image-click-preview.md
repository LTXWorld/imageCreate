# Image Click Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add click-to-enlarge preview for generated images on the workspace and history pages while preserving the existing download flow.

**Architecture:** Add one shared React dialog component for preview display and wire it into `WorkspacePage` and `HistoryPage` with local preview state. The dialog reuses the existing authenticated image URL, closes through a button, backdrop click, or `Escape`, and does not change backend APIs or admin audit behavior.

**Tech Stack:** React, TypeScript, Vite, Vitest, React Testing Library, CSS in `web/src/styles/app.css`.

---

## File Structure

- Create `web/src/components/ImagePreviewDialog.tsx`: shared presentational dialog for showing one image and closing it.
- Modify `web/src/pages/WorkspacePage.tsx`: store the current preview image and open the dialog when the successful result image is clicked.
- Modify `web/src/pages/HistoryPage.tsx`: store the current preview image and open the dialog when a successful history preview image is clicked.
- Modify `web/src/styles/app.css`: add clickable image button styles and responsive dialog overlay/image styles.
- Modify `web/src/pages/WorkspacePage.test.tsx`: test that a generated result image opens the preview and closes through `Escape`.
- Modify `web/src/pages/HistoryPage.test.tsx`: test that a history preview opens the dialog and that the existing download link remains unchanged.

## Task 1: Workspace Preview Dialog

**Files:**
- Create: `web/src/components/ImagePreviewDialog.tsx`
- Modify: `web/src/pages/WorkspacePage.tsx`
- Modify: `web/src/styles/app.css`
- Test: `web/src/pages/WorkspacePage.test.tsx`

- [ ] **Step 1: Write the failing workspace test**

Append this test near the successful generation tests in `web/src/pages/WorkspacePage.test.tsx`:

```tsx
test("opens and closes a preview dialog from the generated result image", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(() =>
    jsonResponse({
      task: {
        id: "task-preview",
        prompt: "星空城堡",
        ratio: "16:9",
        size: "1280x720",
        status: "succeeded",
        image_url: "/api/generations/task-preview/image",
        created_at: "2026-04-30T08:00:00Z",
        completed_at: "2026-04-30T08:01:00Z",
      },
    }),
  );

  render(<WorkspacePage user={user} />);

  await userEvent.type(screen.getByLabelText("提示词"), "星空城堡");
  await userEvent.click(screen.getByRole("button", { name: "生成" }));
  await userEvent.click(await screen.findByRole("button", { name: "预览图片：星空城堡" }));

  const dialog = screen.getByRole("dialog", { name: "图片预览" });
  expect(dialog).toBeInTheDocument();
  expect(screen.getByRole("img", { name: "星空城堡" })).toHaveAttribute(
    "src",
    "/api/generations/task-preview/image",
  );

  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "图片预览" })).not.toBeInTheDocument();
});
```

- [ ] **Step 2: Run the workspace test to verify it fails**

Run:

```bash
cd web && npm test -- WorkspacePage.test.tsx -t "opens and closes a preview dialog from the generated result image"
```

Expected: FAIL because no button named `预览图片：星空城堡` exists yet.

- [ ] **Step 3: Create the shared preview dialog component**

Create `web/src/components/ImagePreviewDialog.tsx`:

```tsx
import { useEffect } from "react";

type ImagePreviewDialogProps = {
  alt: string;
  onClose: () => void;
  src: string;
};

export function ImagePreviewDialog({ alt, onClose, src }: ImagePreviewDialogProps) {
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div
      aria-label="图片预览"
      aria-modal="true"
      className="image-preview-overlay"
      onClick={onClose}
      role="dialog"
    >
      <div className="image-preview-dialog" onClick={(event) => event.stopPropagation()}>
        <button
          aria-label="关闭图片预览"
          className="image-preview-close"
          onClick={onClose}
          type="button"
        >
          ×
        </button>
        <img className="image-preview-large" src={src} alt={alt} />
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Wire the component into the workspace page**

In `web/src/pages/WorkspacePage.tsx`, update imports:

```tsx
import { ImagePreviewDialog } from "../components/ImagePreviewDialog";
```

Add a preview state type near existing type declarations:

```tsx
type PreviewImage = {
  alt: string;
  src: string;
};
```

Add state inside `WorkspacePage`:

```tsx
const [previewImage, setPreviewImage] = useState<PreviewImage | null>(null);
```

Replace the successful image markup:

```tsx
<button
  aria-label={`预览图片：${currentTask.prompt}`}
  className="image-preview-trigger"
  onClick={() => setPreviewImage({ alt: currentTask.prompt, src: currentTask.imageUrl })}
  type="button"
>
  <img className="result-preview" src={currentTask.imageUrl} alt={currentTask.prompt} />
</button>
```

Render the dialog before the closing `</section>`:

```tsx
{previewImage ? (
  <ImagePreviewDialog
    alt={previewImage.alt}
    src={previewImage.src}
    onClose={() => setPreviewImage(null)}
  />
) : null}
```

- [ ] **Step 5: Add dialog and trigger styles**

Append these styles near the existing `.result-preview` and history preview styles in `web/src/styles/app.css`:

```css
.image-preview-trigger {
  width: 100%;
  min-width: 0;
  padding: 0;
  border: 0;
  border-radius: 12px;
  background: transparent;
  line-height: 0;
}

.image-preview-trigger:hover .result-preview,
.image-preview-trigger:hover .history-preview {
  border-color: var(--color-gold);
  box-shadow: 0 12px 34px rgba(18, 61, 47, 0.14);
}

.image-preview-trigger:focus-visible {
  outline: 3px solid rgba(30, 96, 74, 0.32);
  outline-offset: 3px;
}

.image-preview-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 28px;
  background: rgba(12, 18, 16, 0.78);
}

.image-preview-dialog {
  position: relative;
  display: flex;
  max-width: min(1120px, 100%);
  max-height: min(760px, 100%);
}

.image-preview-large {
  max-width: calc(100vw - 56px);
  max-height: calc(100vh - 56px);
  border: 1px solid rgba(255, 255, 255, 0.28);
  border-radius: 12px;
  background: #ffffff;
  object-fit: contain;
  box-shadow: 0 28px 80px rgba(0, 0, 0, 0.35);
}

.image-preview-close {
  position: absolute;
  top: -14px;
  right: -14px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 1px solid rgba(255, 255, 255, 0.45);
  border-radius: 8px;
  color: #ffffff;
  background: rgba(22, 24, 22, 0.82);
  font-size: 24px;
  line-height: 1;
}

.image-preview-close:hover {
  color: var(--color-ink);
  background: #ffffff;
}
```

- [ ] **Step 6: Run the workspace test to verify it passes**

Run:

```bash
cd web && npm test -- WorkspacePage.test.tsx -t "opens and closes a preview dialog from the generated result image"
```

Expected: PASS.

## Task 2: History Preview Wiring

**Files:**
- Modify: `web/src/pages/HistoryPage.tsx`
- Modify: `web/src/styles/app.css`
- Test: `web/src/pages/HistoryPage.test.tsx`

- [ ] **Step 1: Write the failing history test**

Extend the existing `renders only tasks returned by the API response` test in `web/src/pages/HistoryPage.test.tsx` after the download link assertions:

```tsx
await userEvent.click(screen.getByRole("button", { name: "预览图片：我的山谷" }));

const dialog = screen.getByRole("dialog", { name: "图片预览" });
expect(dialog).toBeInTheDocument();
expect(screen.getByRole("img", { name: "我的山谷" })).toHaveAttribute(
  "src",
  "/api/generations/task-1/image",
);

await userEvent.click(screen.getByRole("button", { name: "关闭图片预览" }));
expect(screen.queryByRole("dialog", { name: "图片预览" })).not.toBeInTheDocument();
```

Also add `userEvent` to the imports:

```tsx
import userEvent from "@testing-library/user-event";
```

- [ ] **Step 2: Run the history test to verify it fails**

Run:

```bash
cd web && npm test -- HistoryPage.test.tsx -t "renders only tasks returned by the API response"
```

Expected: FAIL because no button named `预览图片：我的山谷` exists yet.

- [ ] **Step 3: Wire the dialog into history**

In `web/src/pages/HistoryPage.tsx`, update imports:

```tsx
import { ImagePreviewDialog } from "../components/ImagePreviewDialog";
```

Add a preview state type near the props type:

```tsx
type PreviewImage = {
  alt: string;
  src: string;
};
```

Add state inside `HistoryPage`:

```tsx
const [previewImage, setPreviewImage] = useState<PreviewImage | null>(null);
```

Replace the successful history image markup:

```tsx
<button
  aria-label={`预览图片：${task.prompt}`}
  className="image-preview-trigger history-preview-trigger"
  onClick={() => setPreviewImage({ alt: task.prompt, src: task.imageUrl })}
  type="button"
>
  <img className="history-preview" src={task.imageUrl} alt={task.prompt} />
</button>
```

Render the dialog before the closing `</section>`:

```tsx
{previewImage ? (
  <ImagePreviewDialog
    alt={previewImage.alt}
    src={previewImage.src}
    onClose={() => setPreviewImage(null)}
  />
) : null}
```

- [ ] **Step 4: Add history trigger layout styles**

Add this CSS near `.history-preview`:

```css
.history-preview-trigger {
  width: 132px;
}
```

Update the mobile media rule that currently targets `.history-preview` so the trigger expands as well:

```css
.history-preview,
.history-preview-trigger {
  width: 100%;
}
```

- [ ] **Step 5: Run the history test to verify it passes**

Run:

```bash
cd web && npm test -- HistoryPage.test.tsx -t "renders only tasks returned by the API response"
```

Expected: PASS.

## Task 3: Full Frontend Verification

**Files:**
- Verify: `web/src/pages/WorkspacePage.test.tsx`
- Verify: `web/src/pages/HistoryPage.test.tsx`
- Verify: `web/src/pages/AdminPage.test.tsx`

- [ ] **Step 1: Run focused page tests**

Run:

```bash
cd web && npm test -- WorkspacePage.test.tsx HistoryPage.test.tsx AdminPage.test.tsx
```

Expected: PASS. Admin tests should continue to prove no admin image links or previews are rendered.

- [ ] **Step 2: Run the frontend test suite**

Run:

```bash
cd web && npm test
```

Expected: PASS.

- [ ] **Step 3: Check changed files**

Run:

```bash
git status --short
```

Expected: only the design doc, implementation plan, and frontend preview files are changed. `.superpowers/` should remain untracked and should not be staged.
