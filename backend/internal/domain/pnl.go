package domain

import (
	"context"
	"time"
)

const (
	PnLAssetRUSD = "rUSD"
	PnLVersion   = "1.0"
)

type PnLUsecase interface {
	GetPnL(ctx context.Context, walletAddress string, fromRaw string, toRaw string) (*PnLReport, error)
}

type PnLRepository interface {
	GetPnL(ctx context.Context, request PnLReportRequest) (*PnLReport, error)
}

type PnLReportRequest struct {
	WalletAddress string
	From          *time.Time
	To            time.Time
}

type PnLReport struct {
	Agent       PnLAgent            `json:"agent"`
	Period      PnLPeriod           `json:"period"`
	Asset       string              `json:"asset"`
	Summary     PnLSummary          `json:"summary"`
	Revenue     []PnLAccountSummary `json:"revenue"`
	Expenses    []PnLAccountSummary `json:"expenses"`
	AuditTrail  PnLAuditTrail       `json:"auditTrail"`
	GeneratedAt string              `json:"generatedAt"`
	Version     string              `json:"version"`
}

type PnLAgent struct {
	Address      string `json:"address"`
	RegisteredAt string `json:"registeredAt"`
}

type PnLPeriod struct {
	From *string `json:"from"`
	To   string  `json:"to"`
}

type PnLSummary struct {
	TotalRevenue     string `json:"totalRevenue"`
	TotalExpenses    string `json:"totalExpenses"`
	NetIncome        string `json:"netIncome"`
	TransactionCount int64  `json:"transactionCount"`
}

type PnLAccountSummary struct {
	Account    string `json:"account"`
	Amount     string `json:"amount"`
	EntryCount int64  `json:"entryCount"`
}

type PnLAuditTrail struct {
	JournalBatchCount int             `json:"journalBatchCount"`
	Batches           []PnLAuditBatch `json:"batches"`
}

type PnLAuditBatch struct {
	BatchID         string  `json:"batchId"`
	StorageRootHash *string `json:"storageRootHash"`
	EntryCount      int64   `json:"entryCount"`
	AnchoredAt      string  `json:"anchoredAt"`
	ExplorerURL     *string `json:"explorerUrl"`
}
