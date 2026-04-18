package domain

import (
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
	PayerID        uuid.UUID           `json:"payer_id"`
	PayeeID        uuid.UUID           `json:"payee_id"`
	Amount         big.Int             `json:"amount"`
	Status         PaymentIntentStatus `json:"status"`
	NettingBatchID *uuid.UUID          `json:"netting_batch_id"`
	CreatedAt      time.Time           `json:"created_at"`
}
