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
	LedgerInviteGrant      = "invite_grant"
	LedgerAdminAdjustment  = "admin_adjustment"
	LedgerGenerationDebit  = "generation_debit"
	LedgerGenerationRefund = "generation_refund"
)
