package domain

import (
	"math/big"
	"time"

	"github.com/google/uuid"
)

type WorkOrderStatus string

const (
	WorkOrderStatusDraft     WorkOrderStatus = "draft"
	WorkOrderStatusLocked    WorkOrderStatus = "locked"
	WorkOrderStatusCompleted WorkOrderStatus = "completed"
	WorkOrderStatusRefunded  WorkOrderStatus = "refunded"
)

type WorkOrder struct {
	ID             uuid.UUID       `json:"id"`
	IdempotencyKey string          `json:"idempotency_key"`
	CreatorID      uuid.UUID       `json:"creator_id"`
	ProviderID     uuid.UUID       `json:"provider_id"`
	Amount         big.Int         `json:"amount"`
	Status         WorkOrderStatus `json:"status"`
	CriteriaCID    string          `json:"criteria_cid"`
	DeliverableCID *string         `json:"deliverable_cid"`
	FundedAt       *time.Time      `json:"funded_at"`
	CompletedAt    *time.Time      `json:"completed_at"`
	RefundedAt     *time.Time      `json:"refunded_at"`
	OnchainOrderID *big.Int        `json:"onchain_order_id"`
	FundingTxHash  *string         `json:"funding_tx_hash"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
