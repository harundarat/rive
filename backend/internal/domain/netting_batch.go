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
	ID                      uuid.UUID   `json:"id"`
	BatchStatus             BatchStatus `json:"batch_status"`
	BatchHash               *string     `json:"batch_hash"`
	ManifestCID             *string     `json:"manifest_cid"`
	ManifestTxHash          *string     `json:"manifest_tx_hash"`
	SettlementTxHash        *string     `json:"settlement_tx_hash"`
	FailureReason           *string     `json:"failure_reason"`
	GrossIntentCount        int         `json:"gross_intent_count"`
	SettlementTransferCount int         `json:"settlement_transfer_count"`
	GrossAmount             big.Int     `json:"gross_amount"`
	NetAmount               big.Int     `json:"net_amount"`
	TotalGasSaved           big.Int     `json:"total_gas_saved"`
	CreatedAt               time.Time   `json:"created_at"`
	UpdatedAt               time.Time   `json:"updated_at"`
	WindowStart             *time.Time  `json:"window_start"`
	WindowEnd               *time.Time  `json:"window_end"`
}

type NettingBatchClaim struct {
	Batch   NettingBatch    `json:"batch"`
	Intents []PaymentIntent `json:"intents"`
}

type NettingPartyAmount struct {
	AgentID uuid.UUID `json:"agent_id"`
	Wallet  string    `json:"wallet"`
	Amount  big.Int   `json:"amount"`
}

type NettingSettlementInstruction struct {
	BatchHash     string               `json:"batch_hash"`
	Debtors       []NettingPartyAmount `json:"debtors"`
	Creditors     []NettingPartyAmount `json:"creditors"`
	SkipOnchainTx bool                 `json:"skip_onchain_tx"`
}

type NettingSettlementReceipt struct {
	TransactionHash *string `json:"transaction_hash"`
}

type NettingBatchSettlement struct {
	BatchID                 uuid.UUID
	BatchHash               string
	ManifestCID             string
	ManifestTxHash          string
	SettlementTxHash        *string
	GrossIntentCount        int
	SettlementTransferCount int
	GrossAmount             big.Int
	NetAmount               big.Int
	Debtors                 []NettingPartyAmount
	Creditors               []NettingPartyAmount
	SettledAt               time.Time
}

type NettingBatchManifest struct {
	Version          string                    `json:"version"`
	BatchID          uuid.UUID                 `json:"batchId"`
	CreatedAt        string                    `json:"createdAt"`
	Asset            string                    `json:"asset"`
	GrossIntentCount int                       `json:"grossIntentCount"`
	GrossAmount      string                    `json:"grossAmount"`
	NetAmount        string                    `json:"netAmount"`
	Debtors          []NettingManifestPosition `json:"debtors"`
	Creditors        []NettingManifestPosition `json:"creditors"`
	Intents          []NettingManifestIntent   `json:"intents"`
}

type NettingManifestPosition struct {
	Agent  string `json:"agent"`
	Amount string `json:"amount"`
}

type NettingManifestIntent struct {
	ID     uuid.UUID `json:"id"`
	Payer  string    `json:"payer"`
	Payee  string    `json:"payee"`
	Amount string    `json:"amount"`
}
