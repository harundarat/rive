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
	BatchStatusFailed     BatchStatus = "failed"
)

type NettingBatch struct {
	ID               uuid.UUID   `json:"id"`
	BatchStatus      BatchStatus `json:"batch_status"`
	SettlementTxHash *string     `json:"settlement_tx_hash"`
	TotalGasSaved    big.Int     `json:"total_gas_saved"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	WindowStart      *time.Time  `json:"window_start"`
	WindowEnd        *time.Time  `json:"window_end"`
}
