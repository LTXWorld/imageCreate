# Admin Generation Observability Design

## Goal

Give administrators a clear view of whether user image generation is succeeding inside the existing admin audit area. The feature should answer two questions quickly:

- Are generation jobs generally succeeding or failing?
- Which specific jobs failed, and why?

## Scope

This change enhances the current admin audit tab. It uses the existing `/api/admin/generation-tasks` response and does not add a new database table, audit-log event type, or backend endpoint.

Included:

- Summary metrics above the generation task table.
- Clear Chinese status labels for task states.
- Failure reason display in task rows.
- Average latency based only on completed tasks with positive latency.

Excluded:

- Filtering by user, status, or date range.
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

The generation task table keeps its current columns, but the status column becomes easier to scan:

- `succeeded` -> `成功`
- `failed` -> `失败`
- `queued` -> `排队中`
- `running` -> `生成中`
- `canceled` -> `已取消`

When a task failed, the row shows the failure reason. It prefers `error_message`; if that is empty, it uses `error_code`; if both are empty, it shows `-`.

## Data Flow

The admin page already loads generation tasks together with users, invites, and audit logs. The frontend will derive summary metrics from `generationTasks` after normalization:

- `total`: all loaded tasks.
- `succeeded`: tasks whose status is `succeeded`.
- `failed`: tasks whose status is `failed`.
- `active`: tasks whose status is `queued` or `running`.
- `successRate`: `succeeded / (succeeded + failed + canceled)` when at least one terminal task exists; otherwise `0%`.
- `averageLatencyMs`: average latency from tasks with `completedAt` and `latencyMs > 0`.

This keeps the initial implementation small and matches the current unpaginated admin task list.

## Error Handling

If the admin task API fails, the page keeps using the existing global admin load error. Empty task lists render zeroed metrics and an empty table.

Failure reasons are treated as optional because some failed records might have no upstream detail.

## Testing

Add or update frontend tests for the admin page to verify:

- Summary metrics render from mocked generation tasks.
- Success rate ignores queued and running tasks.
- Average latency ignores incomplete or zero-latency tasks.
- Failed rows display the best available failure reason.

No backend tests are required for this design because the API contract is unchanged.
