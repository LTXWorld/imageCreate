# Admin Generation Task Filters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let administrators filter generation task audit rows and metrics by user and task status.

**Architecture:** Keep filtering on the admin page because `/api/admin/generation-tasks` already returns the fields needed for this small dataset. Add local React state for selected user and status, derive a filtered task array, and render metrics/table rows from that array.

**Tech Stack:** React, TypeScript, Vitest, Testing Library.

---

### Task 1: Add Filter Tests

**Files:**
- Modify: `web/src/pages/AdminPage.test.tsx`

- [ ] **Step 1: Expand mocked users and tasks**

In `mockAdminFetch`, add a second user and two tasks owned by that user:

```ts
{
  id: "user-2",
  username: "bob",
  role: "user",
  status: "active",
  credit_balance: 2,
  daily_free_credit_limit: 5,
  daily_free_credit_balance: 2,
  paid_credit_balance: 0,
  created_at: "2026-04-30T09:00:00Z",
  updated_at: "2026-04-30T09:00:00Z",
}
```

```ts
{
  id: "task-5",
  user_id: "user-2",
  username: "bob",
  prompt: "bob 的成功海报",
  size: "1024x1024",
  status: "succeeded",
  latency_ms: 500,
  created_at: "2026-04-30T09:02:00Z",
  completed_at: "2026-04-30T09:03:00Z",
},
{
  id: "task-6",
  user_id: "user-2",
  username: "bob",
  prompt: "bob 的失败海报",
  size: "1024x1024",
  status: "failed",
  latency_ms: 700,
  error_code: "policy_blocked",
  created_at: "2026-04-30T09:04:00Z",
  completed_at: "2026-04-30T09:05:00Z",
}
```

- [ ] **Step 2: Add a test for user and status filters**

Append this test:

```ts
test("filters generation task metrics and rows by user and status", async () => {
  mockAdminFetch();

  render(<AdminPage user={adminUser} />);

  await userEvent.click(await screen.findByRole("tab", { name: "审计" }));
  const taskAudit = await screen.findByLabelText("任务审计");

  await userEvent.selectOptions(within(taskAudit).getByLabelText("筛选用户"), "user-2");
  await userEvent.selectOptions(within(taskAudit).getByLabelText("筛选状态"), "failed");

  const summary = within(taskAudit).getByLabelText("生图结果汇总");
  const totalMetric = within(summary).getByText("总任务").closest(".admin-metric");
  const failedMetric = within(summary).getByText("失败数").closest(".admin-metric");
  const successRateMetric = within(summary).getByText("成功率").closest(".admin-metric");

  expect(totalMetric).not.toBeNull();
  expect(failedMetric).not.toBeNull();
  expect(successRateMetric).not.toBeNull();
  expect(within(totalMetric as HTMLElement).getByText("1")).toBeInTheDocument();
  expect(within(failedMetric as HTMLElement).getByText("1")).toBeInTheDocument();
  expect(within(successRateMetric as HTMLElement).getByText("0%")).toBeInTheDocument();

  expect(within(taskAudit).getByText("bob 的失败海报")).toBeInTheDocument();
  expect(within(taskAudit).queryByText("bob 的成功海报")).not.toBeInTheDocument();
  expect(within(taskAudit).queryByText("失败的森林")).not.toBeInTheDocument();
  expect(within(taskAudit).getByText("policy_blocked")).toBeInTheDocument();
});
```

- [ ] **Step 3: Add a test for empty filtered results**

Append this test:

```ts
test("shows an empty task message when generation filters match no rows", async () => {
  mockAdminFetch();

  render(<AdminPage user={adminUser} />);

  await userEvent.click(await screen.findByRole("tab", { name: "审计" }));
  const taskAudit = await screen.findByLabelText("任务审计");

  await userEvent.selectOptions(within(taskAudit).getByLabelText("筛选用户"), "user-2");
  await userEvent.selectOptions(within(taskAudit).getByLabelText("筛选状态"), "running");

  expect(within(taskAudit).getByText("暂无匹配任务")).toBeInTheDocument();
  expect(within(taskAudit).queryByText("bob 的成功海报")).not.toBeInTheDocument();
});
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `npm --prefix web test -- AdminPage.test.tsx --runInBand`

Expected: the new tests fail because `筛选用户` and `筛选状态` controls do not exist.

### Task 2: Implement Admin Task Filters

**Files:**
- Modify: `web/src/pages/AdminPage.tsx`
- Modify: `web/src/styles/app.css` if spacing for the new filter row needs a small adjustment

- [ ] **Step 1: Add filter types and options**

Near the existing tab definitions, add:

```ts
type GenerationStatusFilter = AdminGenerationTask["status"] | "all";

const generationStatusFilterOptions: Array<{ value: GenerationStatusFilter; label: string }> = [
  { value: "all", label: "全部状态" },
  { value: "succeeded", label: "成功" },
  { value: "failed", label: "失败" },
  { value: "queued", label: "排队中" },
  { value: "running", label: "生成中" },
  { value: "canceled", label: "已取消" },
];
```

- [ ] **Step 2: Add component state and filtered tasks**

Inside `AdminPage`, add:

```ts
const [generationUserFilter, setGenerationUserFilter] = useState("all");
const [generationStatusFilter, setGenerationStatusFilter] = useState<GenerationStatusFilter>("all");
```

Replace:

```ts
const generationSummary = summarizeGenerationTasks(generationTasks);
```

with:

```ts
const filteredGenerationTasks = generationTasks.filter((task) => {
  const matchesUser = generationUserFilter === "all" || task.userId === generationUserFilter;
  const matchesStatus = generationStatusFilter === "all" || task.status === generationStatusFilter;
  return matchesUser && matchesStatus;
});
const generationSummary = summarizeGenerationTasks(filteredGenerationTasks);
```

- [ ] **Step 3: Render filter controls**

In the task audit section, after the metrics and before the table, render:

```tsx
<div className="admin-filters" aria-label="任务筛选">
  <label className="field compact-field">
    <span>用户</span>
    <select
      aria-label="筛选用户"
      onChange={(event) => setGenerationUserFilter(event.target.value)}
      value={generationUserFilter}
    >
      <option value="all">全部用户</option>
      {users.map((item) => (
        <option key={item.id} value={item.id}>
          {item.username}
        </option>
      ))}
    </select>
  </label>
  <label className="field compact-field">
    <span>状态</span>
    <select
      aria-label="筛选状态"
      onChange={(event) => setGenerationStatusFilter(event.target.value as GenerationStatusFilter)}
      value={generationStatusFilter}
    >
      {generationStatusFilterOptions.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  </label>
</div>
```

- [ ] **Step 4: Use filtered rows and empty state**

Replace `generationTasks.map` with `filteredGenerationTasks.map`.

After the mapped rows, render:

```tsx
{filteredGenerationTasks.length === 0 ? (
  <tr>
    <td colSpan={7}>暂无匹配任务</td>
  </tr>
) : null}
```

- [ ] **Step 5: Add filter spacing only if needed**

If there is no existing class that lays out compact form controls, add this CSS:

```css
.admin-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin: 18px 0 12px;
}

.compact-field {
  max-width: 220px;
  min-width: 180px;
}
```

- [ ] **Step 6: Run targeted tests**

Run: `npm --prefix web test -- AdminPage.test.tsx --runInBand`

Expected: all AdminPage tests pass.

- [ ] **Step 7: Run frontend test suite**

Run: `npm --prefix web test -- --runInBand`

Expected: all frontend tests pass.
