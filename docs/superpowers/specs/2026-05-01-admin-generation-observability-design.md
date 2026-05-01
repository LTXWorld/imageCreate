# Admin Generation Observability Design

## Goal

Give administrators a clear view of whether user image generation is succeeding inside the existing admin audit area. The feature should answer two questions quickly:

- Are generation jobs generally succeeding or failing?
- Which specific jobs failed, and why?

## Scope

This change enhances the current admin audit tab. It uses the existing `/api/admin/generation-tasks` response and does not add a new database table, audit-log event type, or backend endpoint.

Included:

- Summary metrics above the generation task table.
- User and status filters for finding a user's specific generation attempts.
- Clear Chinese status labels for task states.
- Failure reason display in task rows.
- Average latency based only on completed tasks with positive latency.

Excluded:

- Date range filtering.
- Pagination or server-side aggregation.
- Writing generation success or failure into `audit_logs`.

## User Experience

On the admin audit tab, the task audit section shows a compact metric row before the table:

- Total tasks.
- Succeeded tasks.
- Failed tasks.
- Active tasks, covering queued and running tasks.
- Success rate.
- Average latency.

Below the metrics and above the table, administrators can narrow the task list with two controls:

- A user select with `全部用户` plus usernames loaded from the existing admin user list.
- A status select with `全部状态`, `成功`, `失败`, `排队中`, `生成中`, and `已取消`.

The selected filters affect both the metric row and the task table, so an administrator can choose one user and `失败` to see that user's failed attempts and failure details.

The generation task table keeps its current columns, but the status column becomes easier to scan:

- `succeeded` -> `成功`
- `failed` -> `失败`
- `queued` -> `排队中`
- `running` -> `生成中`
- `canceled` -> `已取消`

When a task failed, the row shows the failure reason. It prefers `error_message`; if that is empty, it uses `error_code`; if both are empty, it shows `-`.

## Data Flow

The admin page already loads generation tasks together with users, invites, and audit logs. The frontend will derive a filtered task list from `generationTasks`:

- `selectedGenerationUserID === "all"` keeps all users; otherwise it matches `task.userId`.
- `selectedGenerationStatus === "all"` keeps all statuses; otherwise it matches `task.status`.

Summary metrics and table rows are derived from the filtered task list:

- `total`: all loaded tasks.
- `succeeded`: tasks whose status is `succeeded`.
- `failed`: tasks whose status is `failed`.
- `active`: tasks whose status is `queued` or `running`.
- `successRate`: `succeeded / (succeeded + failed + canceled)` when at least one terminal task exists; otherwise `0%`.
- `averageLatencyMs`: average latency from tasks with `completedAt` and `latencyMs > 0`.

This keeps the initial implementation small and matches the current unpaginated admin task list.

## Error Handling

If the admin task API fails, the page keeps using the existing global admin load error. Empty task lists render zeroed metrics and an empty table.

If the filters match no tasks, the table shows a single `暂无匹配任务` row.

Failure reasons are treated as optional because some failed records might have no upstream detail.

## Testing

Add or update frontend tests for the admin page to verify:

- Summary metrics render from mocked generation tasks.
- Success rate ignores queued and running tasks.
- Average latency ignores incomplete or zero-latency tasks.
- Failed rows display the best available failure reason.
- User and status filters narrow the table and summary metrics.

No backend tests are required for this design because the API contract is unchanged.
