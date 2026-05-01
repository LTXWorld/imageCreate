# Admin Generation Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add admin audit metrics that show whether user image generation is succeeding and make failed generation rows easier to diagnose.

**Architecture:** Keep the backend unchanged and derive metrics from the already-loaded `generationTasks` array in `AdminPage`. Add small pure helper functions inside `web/src/pages/AdminPage.tsx` for task labels, failure reasons, latency formatting, and summary calculation, then render a compact metric row above the existing task audit table.

**Tech Stack:** React 18, TypeScript, Vite, Vitest, React Testing Library.

---

## File Structure

- Modify `web/src/pages/AdminPage.test.tsx`: extend the admin fetch mock with mixed generation tasks and add expectations for summary metrics, Chinese status labels, average latency, success rate, and failed row reasons.
- Modify `web/src/pages/AdminPage.tsx`: add helper functions and render the summary metrics plus enhanced task table cells.
- Optionally modify `web/src/styles/app.css`: only if the existing classes cannot present the metric row cleanly.

## Task 1: Failing Frontend Test

**Files:**
- Modify: `web/src/pages/AdminPage.test.tsx`

- [ ] **Step 1: Replace the generation task mock with mixed task data**

In `mockAdminFetch`, replace the `/api/admin/generation-tasks` response body with:

```tsx
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
    {
      id: "task-2",
      user_id: "user-1",
      username: "alice",
      prompt: "失败的森林",
      size: "1024x1024",
      status: "failed",
      latency_ms: 2760,
      error_code: "upstream_error",
      error_message: "上游服务超时",
      created_at: "2026-04-30T08:02:00Z",
      completed_at: "2026-04-30T08:03:00Z",
    },
    {
      id: "task-3",
      user_id: "user-1",
      username: "alice",
      prompt: "排队的海报",
      size: "1024x1024",
      status: "queued",
      latency_ms: 0,
      created_at: "2026-04-30T08:04:00Z",
    },
    {
      id: "task-4",
      user_id: "user-1",
      username: "alice",
      prompt: "取消的头像",
      size: "1024x1024",
      status: "canceled",
      latency_ms: 0,
      created_at: "2026-04-30T08:05:00Z",
      completed_at: "2026-04-30T08:06:00Z",
    },
  ],
});
```

- [ ] **Step 2: Update the existing audit table test expectations**

In `does not render image links in audit task table`, replace the status expectation:

```tsx
expect(screen.getByText("succeeded")).toBeInTheDocument();
```

with:

```tsx
expect(screen.getByText("成功")).toBeInTheDocument();
```

- [ ] **Step 3: Add a failing test for generation observability metrics**

Add this test before the closing `});` of the `describe` block:

```tsx
test("shows generation success metrics and failed task reasons in the audit tab", async () => {
  mockAdminFetch();

  render(<AdminPage user={adminUser} />);

  await userEvent.click(await screen.findByRole("tab", { name: "审计" }));

  const taskAudit = await screen.findByLabelText("任务审计");

  expect(within(taskAudit).getByText("总任务")).toBeInTheDocument();
  expect(within(taskAudit).getByText("4")).toBeInTheDocument();
  expect(within(taskAudit).getByText("成功数")).toBeInTheDocument();
  expect(within(taskAudit).getByText("1")).toBeInTheDocument();
  expect(within(taskAudit).getByText("失败数")).toBeInTheDocument();
  expect(within(taskAudit).getByText("进行中")).toBeInTheDocument();
  expect(within(taskAudit).getByText("成功率")).toBeInTheDocument();
  expect(within(taskAudit).getByText("33%")).toBeInTheDocument();
  expect(within(taskAudit).getByText("平均耗时")).toBeInTheDocument();
  expect(within(taskAudit).getByText("2000 ms")).toBeInTheDocument();

  const failedRow = within(taskAudit).getByRole("row", { name: /失败的森林/ });
  expect(within(failedRow).getByText("失败")).toBeInTheDocument();
  expect(within(failedRow).getByText("上游服务超时")).toBeInTheDocument();

  const queuedRow = within(taskAudit).getByRole("row", { name: /排队的海报/ });
  expect(within(queuedRow).getByText("排队中")).toBeInTheDocument();
});
```

- [ ] **Step 4: Run the test and verify it fails for the missing feature**

Run:

```bash
cd web && npm test -- AdminPage.test.tsx
```

Expected: FAIL because `任务审计` is not an accessible label yet or the metric labels/status text are not rendered.

## Task 2: Implement Admin Audit Metrics

**Files:**
- Modify: `web/src/pages/AdminPage.tsx`
- Optionally modify: `web/src/styles/app.css`

- [ ] **Step 1: Add helper functions below `metadataText`**

Insert:

```tsx
function taskStatusLabel(status: AdminGenerationTask["status"]) {
  const labels: Record<AdminGenerationTask["status"], string> = {
    queued: "排队中",
    running: "生成中",
    succeeded: "成功",
    failed: "失败",
    canceled: "已取消",
  };
  return labels[status] ?? status;
}

function taskFailureReason(task: AdminGenerationTask) {
  if (task.status !== "failed") return "-";
  return task.errorMessage || task.errorCode || "-";
}

function formatLatency(ms: number) {
  return ms > 0 ? `${ms} ms` : "-";
}

function summarizeGenerationTasks(tasks: AdminGenerationTask[]) {
  const completedWithLatency = tasks.filter((task) => task.completedAt && task.latencyMs > 0);
  const latencyTotal = completedWithLatency.reduce((total, task) => total + task.latencyMs, 0);
  const succeeded = tasks.filter((task) => task.status === "succeeded").length;
  const failed = tasks.filter((task) => task.status === "failed").length;
  const canceled = tasks.filter((task) => task.status === "canceled").length;
  const active = tasks.filter((task) => task.status === "queued" || task.status === "running").length;
  const terminal = succeeded + failed + canceled;

  return {
    total: tasks.length,
    succeeded,
    failed,
    active,
    successRate: terminal > 0 ? Math.round((succeeded / terminal) * 100) : 0,
    averageLatencyMs: completedWithLatency.length > 0 ? Math.round(latencyTotal / completedWithLatency.length) : 0,
  };
}
```

- [ ] **Step 2: Derive the summary inside `AdminPage`**

After state declarations and before `useEffect`, add:

```tsx
const generationSummary = summarizeGenerationTasks(generationTasks);
```

- [ ] **Step 3: Make the task audit section accessible and render metric cards**

Change:

```tsx
<section className="admin-section panel" aria-labelledby="task-audit-title">
```

to:

```tsx
<section className="admin-section panel" aria-label="任务审计">
```

Then insert this block immediately after `<h3 id="task-audit-title">任务审计</h3>`:

```tsx
<div className="admin-metrics" aria-label="生图结果汇总">
  <div className="admin-metric">
    <span>总任务</span>
    <strong>{generationSummary.total}</strong>
  </div>
  <div className="admin-metric">
    <span>成功数</span>
    <strong>{generationSummary.succeeded}</strong>
  </div>
  <div className="admin-metric">
    <span>失败数</span>
    <strong>{generationSummary.failed}</strong>
  </div>
  <div className="admin-metric">
    <span>进行中</span>
    <strong>{generationSummary.active}</strong>
  </div>
  <div className="admin-metric">
    <span>成功率</span>
    <strong>{generationSummary.successRate}%</strong>
  </div>
  <div className="admin-metric">
    <span>平均耗时</span>
    <strong>{formatLatency(generationSummary.averageLatencyMs)}</strong>
  </div>
</div>
```

- [ ] **Step 4: Add a failure reason column and Chinese status labels**

In the task audit table header, add a reason column after status:

```tsx
<th>失败原因</th>
```

Then change each task row from:

```tsx
<td>{task.status}</td>
<td>{task.size}</td>
<td>{task.latencyMs} ms</td>
```

to:

```tsx
<td>{taskStatusLabel(task.status)}</td>
<td>{taskFailureReason(task)}</td>
<td>{task.size}</td>
<td>{formatLatency(task.latencyMs)}</td>
```

- [ ] **Step 5: Add metric styling if missing**

If `admin-metrics` and `admin-metric` do not already exist in `web/src/styles/app.css`, append:

```css
.admin-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.admin-metric {
  border: 1px solid rgba(15, 23, 42, 0.12);
  border-radius: 8px;
  padding: 0.75rem;
  background: rgba(255, 255, 255, 0.72);
}

.admin-metric span {
  display: block;
  color: #64748b;
  font-size: 0.85rem;
  margin-bottom: 0.25rem;
}

.admin-metric strong {
  color: #0f172a;
  font-size: 1.1rem;
}
```

- [ ] **Step 6: Run the focused test and verify it passes**

Run:

```bash
cd web && npm test -- AdminPage.test.tsx
```

Expected: PASS.

## Task 3: Final Verification

**Files:**
- Verify: `web/src/pages/AdminPage.tsx`
- Verify: `web/src/pages/AdminPage.test.tsx`
- Verify: `web/src/styles/app.css` if modified

- [ ] **Step 1: Run the full frontend test suite**

Run:

```bash
cd web && npm test
```

Expected: PASS.

- [ ] **Step 2: Run the frontend build**

Run:

```bash
cd web && npm run build
```

Expected: PASS with TypeScript and Vite build success.

- [ ] **Step 3: Inspect the diff**

Run:

```bash
git diff -- web/src/pages/AdminPage.tsx web/src/pages/AdminPage.test.tsx web/src/styles/app.css
```

Expected: only admin audit observability changes are present.

- [ ] **Step 4: Commit implementation**

Run:

```bash
git add web/src/pages/AdminPage.tsx web/src/pages/AdminPage.test.tsx web/src/styles/app.css
git commit -m "feat: show admin generation observability"
```
