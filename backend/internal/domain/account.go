package domain

import (
	"math/big"
	"time"

	"github.com/google/uuid"
)

type AccountType string

const (
	AccountTypeAsset     AccountType = "asset"
	AccountTypeLiability AccountType = "liability"
	AccountTypeRevenue   AccountType = "revenue"
	AccountTypeExpense   AccountType = "expense"
	AccountTypeEquity    AccountType = "equity"
)

// Ledger account names shared by more than one bookkeeping flow. An agent's
// "Service Expense"/"Service Revenue" accounts are posted to by both escrow
// settlement (usecase) and netting accrual (postgres netting repository), so the
// names must come from a single source to keep them pointing at the same row.
const (
	AccountNameServiceExpense = "Service Expense"
	AccountNameServiceRevenue = "Service Revenue"
)

type Account struct {
	ID        uuid.UUID   `json:"id"`
	AgentID   uuid.UUID   `json:"agent_id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Balance   big.Int     `json:"balance"`
	CreatedAt time.Time   `json:"created_at"`
	Version   int         `json:"version"`
}
