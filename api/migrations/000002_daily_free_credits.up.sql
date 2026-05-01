ALTER TABLE users
  ADD COLUMN daily_free_credit_limit INTEGER NOT NULL DEFAULT 0 CHECK (daily_free_credit_limit >= 0),
  ADD COLUMN daily_free_credit_balance INTEGER NOT NULL DEFAULT 0 CHECK (daily_free_credit_balance >= 0),
  ADD COLUMN paid_credit_balance INTEGER NOT NULL DEFAULT 0 CHECK (paid_credit_balance >= 0),
  ADD COLUMN last_daily_free_credit_refreshed_on DATE NOT NULL DEFAULT CURRENT_DATE;

UPDATE users
SET daily_free_credit_limit = credit_balance,
    daily_free_credit_balance = credit_balance,
    paid_credit_balance = 0,
    last_daily_free_credit_refreshed_on = CURRENT_DATE;

ALTER TABLE credit_ledger
  ADD COLUMN wallet_type TEXT CHECK (wallet_type IN ('daily_free', 'paid')),
  ADD COLUMN business_date DATE;

ALTER TABLE credit_ledger
  DROP CONSTRAINT credit_ledger_type_check,
  ADD CONSTRAINT credit_ledger_type_check CHECK (
    type IN (
      'invite_grant',
      'admin_adjustment',
      'generation_debit',
      'generation_refund',
      'daily_free_refresh',
      'daily_free_generation_debit',
      'daily_free_generation_refund',
      'paid_generation_debit',
      'paid_generation_refund',
      'paid_admin_adjustment'
    )
  ),
  ADD CONSTRAINT credit_ledger_daily_free_refresh_business_date_check CHECK (
    type <> 'daily_free_refresh' OR business_date IS NOT NULL
  );

CREATE UNIQUE INDEX credit_ledger_one_daily_free_refresh_per_user_date
ON credit_ledger(user_id, business_date)
WHERE type = 'daily_free_refresh';
