package domain

import (
	"math/big"
	"time"

	"github.com/google/uuid"
)

type BatchStatus string

const (
	BatchStatusOpen       BatchStatus = "open"
	BatchStatusProcessing BatchStatus = "processing"
	BatchStatusSettled    BatchStatus = "settled"
)

type NettingBatch struct {
	ID               uuid.UUID   `json:"id"`
	BatchStatus      BatchStatus `json:"batch_status"`
	SettlementTxHash *string     `json:"settlement_tx_hash"`
	TotalSavedGases  big.Int     `json:"total_saved_gases"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}
