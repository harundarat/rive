package domain

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"
)

type WorkOrderStatus string

const (
	WorkOrderStatusDraft     WorkOrderStatus = "draft"
	WorkOrderStatusFunded    WorkOrderStatus = "funded"
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
	SpecHash       string          `json:"spec_hash"`
	SpecVersion    string          `json:"spec_version"`
	SpecTxHash     string          `json:"spec_tx_hash"`
	DeliverableCID *string         `json:"deliverable_cid"`
	CompletedAt    *time.Time      `json:"completed_at"`
	RefundedAt     *time.Time      `json:"refunded_at"`
	OnchainOrderID *big.Int        `json:"onchain_order_id"`
	OrderTxHash    *string         `json:"order_tx_hash"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type OrderCreatedWorkOrderUpdate struct {
	SpecHash        string
	Payer           string
	Payee           string
	Amount          big.Int
	OnchainOrderID  big.Int
	TransactionHash string
	RecordedAt      time.Time
}

type OrderCreatedWorkOrderRollback struct {
	SpecHash        string
	Payer           string
	Payee           string
	Amount          big.Int
	OnchainOrderID  big.Int
	TransactionHash string
	RolledBackAt    time.Time
}

type OrderReleasedWorkOrderUpdate struct {
	Payee          string
	Amount         big.Int
	OnchainOrderID big.Int
	RecordedAt     time.Time
}

type OrderReleasedWorkOrderRollback struct {
	Payee          string
	Amount         big.Int
	OnchainOrderID big.Int
	RolledBackAt   time.Time
}

type OrderRefundedWorkOrderUpdate struct {
	Payer          string
	Amount         big.Int
	OnchainOrderID big.Int
	RecordedAt     time.Time
}

type OrderRefundedWorkOrderRollback struct {
	Payer          string
	Amount         big.Int
	OnchainOrderID big.Int
	RolledBackAt   time.Time
}

type WorkOrderRepository interface {
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*WorkOrder, error)
	Create(ctx context.Context, workOrder WorkOrder) error
	RecordOrderCreated(ctx context.Context, event OrderCreatedWorkOrderUpdate) (bool, error)
	RollbackOrderCreated(ctx context.Context, event OrderCreatedWorkOrderRollback) (bool, error)
	RecordOrderReleased(ctx context.Context, event OrderReleasedWorkOrderUpdate) (bool, error)
	RollbackOrderReleased(ctx context.Context, event OrderReleasedWorkOrderRollback) (bool, error)
	RecordOrderRefunded(ctx context.Context, event OrderRefundedWorkOrderUpdate) (bool, error)
	RollbackOrderRefunded(ctx context.Context, event OrderRefundedWorkOrderRollback) (bool, error)
}

type WorkOrderOnchainEventUsecase interface {
	RecordOrderCreated(ctx context.Context, event OrderCreatedWorkOrderUpdate) (bool, error)
	RollbackOrderCreated(ctx context.Context, event OrderCreatedWorkOrderRollback) (bool, error)
	RecordOrderReleased(ctx context.Context, event OrderReleasedWorkOrderUpdate) (bool, error)
	RollbackOrderReleased(ctx context.Context, event OrderReleasedWorkOrderRollback) (bool, error)
	RecordOrderRefunded(ctx context.Context, event OrderRefundedWorkOrderUpdate) (bool, error)
	RollbackOrderRefunded(ctx context.Context, event OrderRefundedWorkOrderRollback) (bool, error)
}
