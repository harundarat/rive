package domain

import (
	"time"

	"github.com/google/uuid"
)

type JournalEntry struct {
	ID             uuid.UUID  `json:"id"`
	IdempotencyKey string     `json:"idempotency_key"`
	WorkOrderID    *uuid.UUID `json:"work_order_id"`
	Description    string     `json:"description"`
	StorageCID     *string    `json:"storage_cid"`
	NettingBatchID *uuid.UUID `json:"netting_batch_id"`
	CreatedAt      time.Time  `json:"created_at"`
}
