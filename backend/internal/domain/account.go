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

type Account struct {
	ID        uuid.UUID   `json:"id"`
	AgentID   uuid.UUID   `json:"agent_id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Balance   big.Int     `json:"balance"`
	CreatedAt time.Time   `json:"created_at"`
	Version   int         `json:"version"`
}
