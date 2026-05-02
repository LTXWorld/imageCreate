# Admin Daily Free Balance Top-Up Design

## Goal

Let administrators add to a user's remaining free balance for today without changing the user's daily free limit or paid balance.

## Behavior

- Admins can increase `daily_free_credit_balance` for an existing user.
- The top-up is a direct addition to today's remaining free balance.
- The user's `daily_free_credit_limit` does not change.
- The user's `paid_credit_balance` does not change.
- The user's `credit_balance` changes only because the free balance changed.
- The amount must be a positive PostgreSQL integer.
- A successful top-up writes an audit log entry with action `top_up_daily_free_credit_balance`.

## API

Add:

- `PATCH /api/admin/users/{id}/daily-free-balance`

Request:

```json
{ "amount": 3 }
```

Response:

```json
{ "user": { "...": "updated admin user response" } }
```

## UI

In the admin credit management table:

- Keep `今日免费 X/Y` as the display for current free balance and limit.
- Add a compact input/button pair for topping up today's free balance.
- Keep the daily limit control and paid credit adjustment controls separate.

## Testing

Backend tests:

- Admin can top up today's free balance.
- The top-up updates `daily_free_credit_balance` and `credit_balance` only.
- Zero and negative values are rejected.
- An audit log entry is written.

Frontend tests:

- Admin can submit a free-balance top-up for a user.
- The request uses `PATCH /api/admin/users/{id}/daily-free-balance`.
- The updated `今日免费 X/Y` value is rendered after success.

