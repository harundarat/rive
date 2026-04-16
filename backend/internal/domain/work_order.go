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
	WorkOrderStatusDisputed  WorkOrderStatus = "disputed"
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
	NettingBatchID *uuid.UUID      `json:"netting_batch_id"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
