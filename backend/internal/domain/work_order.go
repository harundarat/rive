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
	DeliveredAt    *time.Time      `json:"delivered_at"`
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
	BlockNumber     string
	LogIndex        string
	RecordedAt      time.Time
}

type OrderCreatedWorkOrderRollback struct {
	SpecHash        string
	Payer           string
	Payee           string
	Amount          big.Int
	OnchainOrderID  big.Int
	TransactionHash string
	BlockNumber     string
	LogIndex        string
	RolledBackAt    time.Time
}

type OrderReleasedWorkOrderUpdate struct {
	Payee           string
	Amount          big.Int
	OnchainOrderID  big.Int
	TransactionHash string
	BlockNumber     string
	LogIndex        string
	RecordedAt      time.Time
}

type OrderReleasedWorkOrderRollback struct {
	Payee           string
	Amount          big.Int
	OnchainOrderID  big.Int
	TransactionHash string
	BlockNumber     string
	LogIndex        string
	RolledBackAt    time.Time
}

type OrderRefundedWorkOrderUpdate struct {
	Payer           string
	Amount          big.Int
	OnchainOrderID  big.Int
	TransactionHash string
	BlockNumber     string
	LogIndex        string
	RecordedAt      time.Time
}

type OrderRefundedWorkOrderRollback struct {
	Payer           string
	Amount          big.Int
	OnchainOrderID  big.Int
	TransactionHash string
	BlockNumber     string
	LogIndex        string
	RolledBackAt    time.Time
}

type WorkOrderDeliveryTarget struct {
	WorkOrder WorkOrder
	Payee     string
}

type WorkOrderDeliveryUpdate struct {
	OnchainOrderID big.Int
	DeliveryHash   string
	DeliveredAt    time.Time
}

// WorkOrderBookkeepingTarget identifies the work order and its two counterparties
// that an escrow event resolves to. The usecase resolves this (read-only) before
// uploading the journal anchor to 0G, so the 0G upload never holds a DB transaction.
type WorkOrderBookkeepingTarget struct {
	WorkOrderID uuid.UUID
	PayerID     uuid.UUID
	PayeeID     uuid.UUID
}

// LedgerPosting is one leg of a double-entry escrow journal entry.
type LedgerPosting struct {
	AgentID     uuid.UUID
	AccountName string
	AccountType AccountType
	EntryType   LedgerEntryType
}

// EscrowJournalEntry is a fully-prepared journal entry the usecase hands to the
// repository for atomic persistence. StorageCID is the 0G root hash obtained from
// an upload that already completed outside any DB transaction.
type EscrowJournalEntry struct {
	JournalID      uuid.UUID
	IdempotencyKey string
	WorkOrderID    uuid.UUID
	Description    string
	StorageCID     string
	CreatedAt      time.Time
	Amount         big.Int
	Postings       []LedgerPosting
}

type WorkOrderRepository interface {
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*WorkOrder, error)
	FindByOnchainOrderID(ctx context.Context, onchainOrderID big.Int) (*WorkOrder, error)
	FindDeliveryTargetByOnchainOrderID(ctx context.Context, onchainOrderID big.Int) (*WorkOrderDeliveryTarget, error)
	Create(ctx context.Context, workOrder WorkOrder) error
	SubmitDelivery(ctx context.Context, update WorkOrderDeliveryUpdate) (bool, error)

	// Resolve* methods are read-only: they match the event against persisted state
	// (returning matched=false for a no-op) and yield the IDs the usecase needs to
	// build the journal anchor. Apply* methods perform the state transition together
	// with the prepared journal/ledger writes in a single transaction — with no
	// network I/O held open inside it.
	ResolveOrderCreatedTarget(ctx context.Context, event OrderCreatedWorkOrderUpdate) (*WorkOrderBookkeepingTarget, bool, error)
	ApplyOrderCreated(ctx context.Context, event OrderCreatedWorkOrderUpdate, entry EscrowJournalEntry) (bool, error)
	ResolveOrderCreatedRollbackTarget(ctx context.Context, event OrderCreatedWorkOrderRollback) (*WorkOrderBookkeepingTarget, bool, error)
	ApplyOrderCreatedRollback(ctx context.Context, event OrderCreatedWorkOrderRollback, entry EscrowJournalEntry) (bool, error)
	ResolveOrderReleasedTarget(ctx context.Context, event OrderReleasedWorkOrderUpdate) (*WorkOrderBookkeepingTarget, bool, error)
	ApplyOrderReleased(ctx context.Context, event OrderReleasedWorkOrderUpdate, entry EscrowJournalEntry) (bool, error)
	ResolveOrderReleasedRollbackTarget(ctx context.Context, event OrderReleasedWorkOrderRollback) (*WorkOrderBookkeepingTarget, bool, error)
	ApplyOrderReleasedRollback(ctx context.Context, event OrderReleasedWorkOrderRollback, entry EscrowJournalEntry) (bool, error)
	ResolveOrderRefundedTarget(ctx context.Context, event OrderRefundedWorkOrderUpdate) (*WorkOrderBookkeepingTarget, bool, error)
	ApplyOrderRefunded(ctx context.Context, event OrderRefundedWorkOrderUpdate, entry EscrowJournalEntry) (bool, error)
	ResolveOrderRefundedRollbackTarget(ctx context.Context, event OrderRefundedWorkOrderRollback) (*WorkOrderBookkeepingTarget, bool, error)
	ApplyOrderRefundedRollback(ctx context.Context, event OrderRefundedWorkOrderRollback, entry EscrowJournalEntry) (bool, error)
}

type WorkOrderOnchainEventUsecase interface {
	RecordOrderCreated(ctx context.Context, event OrderCreatedWorkOrderUpdate) (bool, error)
	RollbackOrderCreated(ctx context.Context, event OrderCreatedWorkOrderRollback) (bool, error)
	RecordOrderReleased(ctx context.Context, event OrderReleasedWorkOrderUpdate) (bool, error)
	RollbackOrderReleased(ctx context.Context, event OrderReleasedWorkOrderRollback) (bool, error)
	RecordOrderRefunded(ctx context.Context, event OrderRefundedWorkOrderUpdate) (bool, error)
	RollbackOrderRefunded(ctx context.Context, event OrderRefundedWorkOrderRollback) (bool, error)
}
