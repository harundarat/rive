package domain

import (
	"time"

	"github.com/google/uuid"
)

type JournalEntry struct {
	ID             uuid.UUID `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	WorkOrderID    uuid.UUID `json:"work_order_id"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
	StorageCID     *string   `json:"storage_cid"`
}
