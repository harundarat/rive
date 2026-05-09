package domain

import (
	"context"
	"time"
)

const (
	PnLAssetRUSD     = "rUSD"
	PnLAssetDecimals = 18
	PnLVersion       = "1.0"
)

const (
	PnLAuditSourceEscrowEvent  = "escrow_event"
	PnLAuditSourceNettingBatch = "netting_batch"

	PnLCounterpartyRolePayer = "payer"
	PnLCounterpartyRolePayee = "payee"

	PnLReferenceTypeWorkOrder     = "work_order"
	PnLReferenceTypePaymentIntent = "payment_intent"
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
	Agent        PnLAgent            `json:"agent"`
	Period       PnLPeriod           `json:"period"`
	Asset        string              `json:"asset"`
	Decimals     int                 `json:"decimals"`
	Summary      PnLSummary          `json:"summary"`
	Revenue      []PnLAccountSummary `json:"revenue"`
	Expenses     []PnLAccountSummary `json:"expenses"`
	Transactions []PnLTransaction    `json:"transactions"`
	AuditTrail   PnLAuditTrail       `json:"auditTrail"`
	GeneratedAt  string              `json:"generatedAt"`
	Version      string              `json:"version"`
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
	BatchID          string  `json:"batchId"`
	Source           string  `json:"source"`
	StorageRootHash  *string `json:"storageRootHash"`
	EntryCount       int64   `json:"entryCount"`
	AnchoredAt       string  `json:"anchoredAt"`
	ExplorerURL      *string `json:"explorerUrl"`
	ChainExplorerURL *string `json:"chainExplorerUrl"`
}

type PnLTransaction struct {
	JournalEntryID string          `json:"journalEntryId"`
	AuditBatchID   *string         `json:"auditBatchId"`
	Source         string          `json:"source"`
	Account        string          `json:"account"`
	AccountType    AccountType     `json:"accountType"`
	EntryType      LedgerEntryType `json:"entryType"`
	Amount         string          `json:"amount"`
	Counterparty   PnLCounterparty `json:"counterparty"`
	Reference      PnLReference    `json:"reference"`
	Description    string          `json:"description"`
	OccurredAt     string          `json:"occurredAt"`
}

type PnLCounterparty struct {
	Role    string `json:"role"`
	Address string `json:"address"`
}

type PnLReference struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}
