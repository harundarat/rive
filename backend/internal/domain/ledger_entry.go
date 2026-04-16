package domain

import (
	"math/big"
	"time"

	"github.com/google/uuid"
)

type LedgerEntryType string

const (
	LedgerEntryTypeDebit  LedgerEntryType = "debit"
	LedgerEntryTypeCredit LedgerEntryType = "credit"
)

type LedgerEntry struct {
	ID             uuid.UUID       `json:"id"`
	AccountID      uuid.UUID       `json:"account_id"`
	JournalEntryID uuid.UUID       `json:"journal_entry_id"`
	Amount         big.Int         `json:"amount"`
	EntryType      LedgerEntryType `json:"entry_type"`
	CreatedAt      time.Time       `json:"created_at"`
}
