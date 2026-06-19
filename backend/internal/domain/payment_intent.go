package domain

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"
)

type PaymentIntentStatus string

const (
	PaymentIntentStatusPending PaymentIntentStatus = "pending"
	PaymentIntentStatusBatched PaymentIntentStatus = "batched"
	PaymentIntentStatusSettled PaymentIntentStatus = "settled"
	PaymentIntentStatusFailed  PaymentIntentStatus = "failed"
)

type PaymentIntent struct {
	ID             uuid.UUID           `json:"id"`
	IdempotencyKey string              `json:"idempotency_key"`
	PayerID        uuid.UUID           `json:"payer_id"`
	PayeeID        uuid.UUID           `json:"payee_id"`
	PayerWallet    string              `json:"payer_wallet,omitempty"`
	PayeeWallet    string              `json:"payee_wallet,omitempty"`
	Amount         big.Int             `json:"amount"`
	Asset          string              `json:"asset"`
	Status         PaymentIntentStatus `json:"status"`
	NettingBatchID *uuid.UUID          `json:"netting_batch_id"`
	FailureReason  *string             `json:"failure_reason,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	SettledAt      *time.Time          `json:"settled_at,omitempty"`
}

type PaymentIntentRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	Payer          string `json:"payer"`
	Payee          string `json:"payee"`
	Amount         string `json:"amount"`
	Asset          string `json:"asset"`
}

type PaymentIntentResponse struct {
	ID             uuid.UUID  `json:"id"`
	IdempotencyKey string     `json:"idempotency_key"`
	Payer          string     `json:"payer"`
	Payee          string     `json:"payee"`
	Amount         string     `json:"amount"`
	Asset          string     `json:"asset"`
	Status         string     `json:"status"`
	NettingBatchID *uuid.UUID `json:"netting_batch_id,omitempty"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
	SettledAt      *string    `json:"settled_at,omitempty"`
}

type NettingUsecase interface {
	SubmitIntent(ctx context.Context, request PaymentIntentRequest) (*PaymentIntentResponse, error)
}

type NettingRepository interface {
	FindIntentByIdempotencyKey(ctx context.Context, idempotencyKey string) (*PaymentIntent, error)
	CreateIntentWithAccrual(ctx context.Context, intent PaymentIntent) (*PaymentIntent, error)
	ClaimPendingIntents(ctx context.Context, windowEnd time.Time, batchID uuid.UUID, claimedAt time.Time) (*NettingBatchClaim, error)
	MarkBatchSettled(ctx context.Context, settlement NettingBatchSettlement) error
	MarkBatchFailed(ctx context.Context, batchID uuid.UUID, reason string, failedAt time.Time) error
	FindStuckProcessingBatches(ctx context.Context, olderThan time.Time) ([]uuid.UUID, error)
}

type NettingSettlementGateway interface {
	SettleBatch(ctx context.Context, instruction NettingSettlementInstruction) (*NettingSettlementReceipt, error)
}
