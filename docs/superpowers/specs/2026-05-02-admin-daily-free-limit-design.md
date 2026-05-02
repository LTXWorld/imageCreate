# Admin Daily Free Limit Design

## Goal

Let administrators update a user's daily free credit limit after registration.

## Context

New users get their daily free credit limit from the invite they used during registration. After registration, that per-user limit lives on `users.daily_free_credit_limit`.

Admins can already adjust paid credits through the credit management tab. That path must remain separate from daily free limit changes because paid credits and daily free credits have different business behavior.

## Behavior

- Admins can edit `daily_free_credit_limit` for an existing user.
- The new limit must be a non-negative PostgreSQL integer.
- Updating the limit does not change `daily_free_credit_balance`.
- Updating the limit does not change `paid_credit_balance`.
- Updating the limit does not change `credit_balance` immediately because the current total balance is based on today's free balance plus paid balance.
- If the new limit is below today's remaining free balance, the balance is not truncated. The next daily refresh will restore the free balance to the new limit.
- A successful update writes an audit log entry with action `update_daily_free_credit_limit` and metadata including `daily_free_credit_limit`.

## API

Add:

- `PATCH /api/admin/users/{id}/daily-free-limit`

Request:

```json
{ "daily_free_credit_limit": 7 }
```

Response:

```json
{ "user": { "...": "updated admin user response" } }
```

## UI

In the admin credit management table:

- Show the current free quota as `今日免费 X/Y`.
- Add a compact input for the user's daily free limit.
- Submit that input separately from paid credit adjustment.

This keeps the two admin operations explicit:

- Daily free limit: controls future daily refresh amount.
- Paid credit adjustment: changes spendable paid balance immediately.

## Testing

Backend tests:

- Admin can update a user's daily free limit.
- The update leaves today's free balance, paid balance, and total balance unchanged.
- Zero is accepted.
- Negative values are rejected.
- An audit log entry is written.

Frontend tests:

- Admin can submit a new daily free limit for a user.
- The request uses `PATCH /api/admin/users/{id}/daily-free-limit`.
- The updated user row is rendered after success.

