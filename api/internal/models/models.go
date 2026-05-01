package models

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

const (
	TaskQueued    = "queued"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"
	TaskCanceled  = "canceled"
)

const (
	WalletDailyFree = "daily_free"
	WalletPaid      = "paid"
)

const (
	LedgerInviteGrant               = "invite_grant"
	LedgerAdminAdjustment           = "admin_adjustment"
	LedgerGenerationDebit           = "generation_debit"
	LedgerGenerationRefund          = "generation_refund"
	LedgerDailyFreeRefresh          = "daily_free_refresh"
	LedgerDailyFreeGenerationDebit  = "daily_free_generation_debit"
	LedgerDailyFreeGenerationRefund = "daily_free_generation_refund"
	LedgerPaidGenerationDebit       = "paid_generation_debit"
	LedgerPaidGenerationRefund      = "paid_generation_refund"
	LedgerPaidAdminAdjustment       = "paid_admin_adjustment"
)
