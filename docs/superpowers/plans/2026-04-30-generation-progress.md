# Generation Progress Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a smooth perceived progress bar to the image generation workspace while keeping the existing backend polling contract.

**Architecture:** The backend remains unchanged. `WorkspacePage.tsx` owns a small pure progress calculation helper and a presentational `GenerationProgress` component; active tasks use local one-second ticks for animation while the existing five-second polling still owns true status updates.

**Tech Stack:** React 18, TypeScript, Vite, Vitest, Testing Library, existing CSS in `web/src/styles/app.css`.

---

## File Structure

- Modify `web/src/pages/WorkspacePage.test.tsx`: add red tests for queued, running, succeeded, and failed progress behavior.
- Modify `web/src/pages/WorkspacePage.tsx`: add progress calculation, local tick timer, and active task progress rendering.
- Modify `web/src/styles/app.css`: add progress bar styles consistent with existing workspace card styling.

## Task 1: Add Failing Progress Tests

**Files:**
- Modify: `web/src/pages/WorkspacePage.test.tsx`

- [ ] **Step 1: Add failing queued/running/succeeded/failed progress tests**

Add these tests inside the existing `describe("WorkspacePage", () => { ... })` block:

```tsx
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

    const progress = await screen.findByRole("progressbar", { name: "生成进度" });
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

    expect(await screen.findByRole("progressbar", { name: "生成进度" })).toHaveAttribute("aria-valuenow", "36");
    expect(screen.getByText("正在绘制细节")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(screen.getByRole("progressbar", { name: "生成进度" })).toHaveAttribute("aria-valuenow", "37");
    expect(fetchMock).toHaveBeenCalledTimes(1);
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

  test("keeps failure guidance visible with stable progress", async () => {
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

    await userEvent.type(screen.getByLabelText("提示词"), "海边");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    expect(await screen.findByRole("progressbar", { name: "生成进度" })).toHaveAttribute("aria-valuenow", "47");
    expect(screen.getByText("生成未完成")).toBeInTheDocument();
    expect(screen.getByText("生成失败，已退回 1 点，可调整提示词后重试。")).toBeInTheDocument();
  });
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
npm --prefix web test -- WorkspacePage.test.tsx
```

Expected: FAIL because no `role="progressbar"` exists yet.

## Task 2: Implement Progress Calculation and UI

**Files:**
- Modify: `web/src/pages/WorkspacePage.tsx`
- Modify: `web/src/styles/app.css`

- [ ] **Step 1: Add progress helpers in `WorkspacePage.tsx`**

Add these constants, types, and helpers near the existing `taskPollingIntervalMS` constant:

```tsx
const progressTickIntervalMS = 1000;
const queuedProgressDurationMS = 90_000;
const runningProgressDurationMS = 180_000;

type GenerationProgressState = {
  percent: number;
  label: string;
  helperText: string;
};

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function elapsedTaskMilliseconds(task: GenerationTask, now: number) {
  const createdAt = Date.parse(task.createdAt);
  if (Number.isNaN(createdAt)) return 0;
  return Math.max(0, now - createdAt);
}

function progressBetween(elapsedMS: number, durationMS: number, start: number, end: number) {
  const ratio = clamp(elapsedMS / durationMS, 0, 1);
  return Math.round(start + (end - start) * ratio);
}

function getGenerationProgress(task: GenerationTask, now: number): GenerationProgressState {
  const elapsedMS = elapsedTaskMilliseconds(task, now);

  if (task.status === "succeeded") {
    return {
      percent: 100,
      label: "生成完成",
      helperText: "图片已生成，可以预览或下载。",
    };
  }

  if (task.status === "failed") {
    return {
      percent: progressBetween(elapsedMS, runningProgressDurationMS, 25, 92),
      label: "生成未完成",
      helperText: "本次生成已结束，点数会按失败规则退回。",
    };
  }

  if (task.status === "running") {
    return {
      percent: progressBetween(elapsedMS, runningProgressDurationMS, 25, 92),
      label: elapsedMS > 120_000 ? "即将完成" : "正在绘制细节",
      helperText: "请保持页面打开，完成后会自动显示结果。",
    };
  }

  return {
    percent: progressBetween(elapsedMS, queuedProgressDurationMS, 5, 25),
    label: "正在排队",
    helperText: "任务已提交，系统正在准备生成。",
  };
}
```

- [ ] **Step 2: Add the `GenerationProgress` component**

Add this component above `export function WorkspacePage`:

```tsx
function GenerationProgress({ task, now }: { task: GenerationTask; now: number }) {
  const progress = getGenerationProgress(task, now);

  return (
    <div className="generation-progress" aria-label="生成进度详情">
      <div className="progress-row">
        <strong>{progress.label}</strong>
        <span>{progress.percent}%</span>
      </div>
      <div
        aria-label="生成进度"
        aria-valuemax={100}
        aria-valuemin={0}
        aria-valuenow={progress.percent}
        className="progress-track"
        role="progressbar"
      >
        <span className="progress-fill" style={{ width: `${progress.percent}%` }} />
      </div>
      <p className="muted-text">{progress.helperText}</p>
    </div>
  );
}
```

- [ ] **Step 3: Add local tick state in `WorkspacePage`**

Inside `WorkspacePage`, add:

```tsx
  const [progressNow, setProgressNow] = useState(() => Date.now());
```

Add this effect after the existing polling effect:

```tsx
  useEffect(() => {
    if (!currentTask || !activeStatuses.has(currentTask.status)) return undefined;

    setProgressNow(Date.now());
    const timer = window.setInterval(() => {
      setProgressNow(Date.now());
    }, progressTickIntervalMS);

    return () => window.clearInterval(timer);
  }, [currentTask]);
```

- [ ] **Step 4: Render progress in the task detail**

Replace:

```tsx
              {isActiveTask(currentTask) ? <p className="muted-text">生成中</p> : null}
```

with:

```tsx
              {currentTask.status !== "canceled" ? <GenerationProgress task={currentTask} now={progressNow} /> : null}
```

- [ ] **Step 5: Add CSS**

Add these styles near the task detail styles in `web/src/styles/app.css`:

```css
.generation-progress {
  display: grid;
  gap: 8px;
}

.generation-progress .muted-text {
  margin: 0;
}

.progress-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: #25231f;
  font-size: 14px;
}

.progress-row span {
  color: #7f7463;
  font-weight: 700;
}

.progress-track {
  width: 100%;
  height: 10px;
  overflow: hidden;
  border: 1px solid #ded8cd;
  border-radius: 999px;
  background: #eee9df;
}

.progress-fill {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: #2f6f5e;
  transition: width 220ms ease;
}
```

- [ ] **Step 6: Run focused tests**

Run:

```bash
npm --prefix web test -- WorkspacePage.test.tsx
```

Expected: PASS.

## Task 3: Verify Broader Frontend

**Files:**
- No code changes expected.

- [ ] **Step 1: Run frontend build**

Run:

```bash
npm --prefix web run build
```

Expected: PASS with TypeScript and Vite build completing.

- [ ] **Step 2: Check worktree**

Run:

```bash
git status --short
```

Expected: only the plan file, `WorkspacePage.tsx`, `WorkspacePage.test.tsx`, and `app.css` are modified or added.

- [ ] **Step 3: Commit implementation**

Run:

```bash
git add docs/superpowers/plans/2026-04-30-generation-progress.md web/src/pages/WorkspacePage.tsx web/src/pages/WorkspacePage.test.tsx web/src/styles/app.css
git commit -m "feat: show generation progress"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: active progress UI, perceived progress calculation, unchanged backend contract, local tick timer, terminal states, styling, and frontend tests are all covered.
- Placeholder scan: no placeholder tasks remain.
- Type consistency: helper names and task fields match the existing `GenerationTask` type and planned JSX usage.
