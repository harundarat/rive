package postgres

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pnlExplorerBaseURL      = "https://storagescan.0g.ai/tx/"
	pnlChainExplorerBaseURL = "https://chainscan.0g.ai/tx/"
)

type pnlDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type PnLRepository struct {
	db pnlDB
}

func NewPnLRepository(db *pgxpool.Pool) *PnLRepository {
	return &PnLRepository{db: db}
}

func (r *PnLRepository) GetPnL(ctx context.Context, request domain.PnLReportRequest) (*domain.PnLReport, error) {
	agentID, agentAddress, registeredAt, err := r.findPnLAgent(ctx, request.WalletAddress)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	revenue, expenses, totalRevenue, totalExpenses, err := r.fetchPnLAccountSummaries(ctx, agentID, request)
	if err != nil {
		return nil, err
	}

	transactionCount, err := r.countPnLTransactions(ctx, agentID, request)
	if err != nil {
		return nil, err
	}

	batches, err := r.fetchPnLAuditBatches(ctx, agentID, request)
	if err != nil {
		return nil, err
	}

	transactions, err := r.fetchPnLTransactions(ctx, agentID, request)
	if err != nil {
		return nil, err
	}

	netIncome := new(big.Int).Sub(totalRevenue, totalExpenses)

	return &domain.PnLReport{
		Agent: domain.PnLAgent{
			Address:      agentAddress,
			RegisteredAt: registeredAt.UTC().Format(time.RFC3339),
		},
		Asset:    domain.PnLAssetRUSD,
		Decimals: domain.PnLAssetDecimals,
		Summary: domain.PnLSummary{
			TotalRevenue:     totalRevenue.String(),
			TotalExpenses:    totalExpenses.String(),
			NetIncome:        netIncome.String(),
			TransactionCount: transactionCount,
		},
		Revenue:      revenue,
		Expenses:     expenses,
		Transactions: transactions,
		AuditTrail: domain.PnLAuditTrail{
			JournalBatchCount: len(batches),
			Batches:           batches,
		},
	}, nil
}

func (r *PnLRepository) findPnLAgent(ctx context.Context, walletAddress string) (uuid.UUID, string, time.Time, error) {
	var id uuid.UUID
	var address string
	var registeredAt time.Time
	err := r.db.QueryRow(ctx, `
		SELECT id, wallet_address, created_at
		FROM agents
		WHERE LOWER(wallet_address) = LOWER($1)
	`, walletAddress).Scan(&id, &address, &registeredAt)
	if err != nil {
		return uuid.Nil, "", time.Time{}, fmt.Errorf("find pnl agent: %w", err)
	}

	return id, address, registeredAt, nil
}

func (r *PnLRepository) fetchPnLAccountSummaries(
	ctx context.Context,
	agentID uuid.UUID,
	request domain.PnLReportRequest,
) ([]domain.PnLAccountSummary, []domain.PnLAccountSummary, *big.Int, *big.Int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			accounts.type,
			accounts.name,
			SUM(CASE
				WHEN accounts.type = 'revenue' AND ledger_entries.entry_type = 'credit' THEN ledger_entries.amount
				WHEN accounts.type = 'revenue' AND ledger_entries.entry_type = 'debit' THEN -ledger_entries.amount
				WHEN accounts.type = 'expense' AND ledger_entries.entry_type = 'debit' THEN ledger_entries.amount
				WHEN accounts.type = 'expense' AND ledger_entries.entry_type = 'credit' THEN -ledger_entries.amount
				ELSE 0
			END)::text AS amount,
			COUNT(*)::bigint AS entry_count
		FROM ledger_entries
		INNER JOIN accounts ON accounts.id = ledger_entries.account_id
		INNER JOIN journal_entries ON journal_entries.id = ledger_entries.journal_entry_id
		WHERE accounts.agent_id = $1
			AND accounts.type IN ('revenue', 'expense')
			AND ($2::timestamptz IS NULL OR ledger_entries.created_at >= $2::timestamptz)
			AND ledger_entries.created_at < $3::timestamptz
		GROUP BY accounts.type, accounts.name
		ORDER BY accounts.type, accounts.name
	`, agentID, pnlTimeArg(request.From), request.To)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("query pnl account summaries: %w", err)
	}
	defer rows.Close()

	revenue := []domain.PnLAccountSummary{}
	expenses := []domain.PnLAccountSummary{}
	totalRevenue := big.NewInt(0)
	totalExpenses := big.NewInt(0)
	for rows.Next() {
		var accountType string
		var accountName string
		var amountText string
		var entryCount int64
		if err := rows.Scan(&accountType, &accountName, &amountText, &entryCount); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("scan pnl account summary: %w", err)
		}

		amount, ok := new(big.Int).SetString(amountText, 10)
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("invalid pnl amount: %q", amountText)
		}

		summary := domain.PnLAccountSummary{
			Account:    accountName,
			Amount:     amount.String(),
			EntryCount: entryCount,
		}
		switch domain.AccountType(accountType) {
		case domain.AccountTypeRevenue:
			revenue = append(revenue, summary)
			totalRevenue.Add(totalRevenue, amount)
		case domain.AccountTypeExpense:
			expenses = append(expenses, summary)
			totalExpenses.Add(totalExpenses, amount)
		default:
			return nil, nil, nil, nil, fmt.Errorf("unexpected pnl account type: %s", accountType)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("iterate pnl account summaries: %w", err)
	}

	return revenue, expenses, totalRevenue, totalExpenses, nil
}

func (r *PnLRepository) countPnLTransactions(ctx context.Context, agentID uuid.UUID, request domain.PnLReportRequest) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT ledger_entries.journal_entry_id)::bigint
		FROM ledger_entries
		INNER JOIN accounts ON accounts.id = ledger_entries.account_id
		INNER JOIN journal_entries ON journal_entries.id = ledger_entries.journal_entry_id
		WHERE accounts.agent_id = $1
			AND accounts.type IN ('revenue', 'expense')
			AND ($2::timestamptz IS NULL OR ledger_entries.created_at >= $2::timestamptz)
			AND ledger_entries.created_at < $3::timestamptz
	`, agentID, pnlTimeArg(request.From), request.To).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pnl transactions: %w", err)
	}

	return count, nil
}

func (r *PnLRepository) fetchPnLAuditBatches(ctx context.Context, agentID uuid.UUID, request domain.PnLReportRequest) ([]domain.PnLAuditBatch, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			audit_batches.batch_id,
			audit_batches.source,
			audit_batches.storage_root_hash,
			audit_batches.entry_count,
			audit_batches.anchored_at,
			audit_batches.chain_tx_hash
		FROM (
			SELECT
				netting_batches.id::text AS batch_id,
				'netting_batch' AS source,
				netting_batches.batch_hash AS storage_root_hash,
				COUNT(DISTINCT journal_entries.id)::bigint AS entry_count,
				netting_batches.updated_at AS anchored_at,
				netting_batches.settlement_tx_hash AS chain_tx_hash
			FROM ledger_entries
			INNER JOIN accounts ON accounts.id = ledger_entries.account_id
			INNER JOIN journal_entries ON journal_entries.id = ledger_entries.journal_entry_id
			INNER JOIN netting_batches ON netting_batches.id = journal_entries.netting_batch_id
			WHERE accounts.agent_id = $1
				AND accounts.type IN ('revenue', 'expense')
				AND netting_batches.batch_status = 'settled'
				AND ($2::timestamptz IS NULL OR ledger_entries.created_at >= $2::timestamptz)
				AND ledger_entries.created_at < $3::timestamptz
			GROUP BY netting_batches.id, netting_batches.batch_hash, netting_batches.settlement_tx_hash, netting_batches.updated_at

			UNION ALL

			SELECT
				journal_entries.id::text AS batch_id,
				'escrow_event' AS source,
				journal_entries.storage_cid AS storage_root_hash,
				1::bigint AS entry_count,
				journal_entries.created_at AS anchored_at,
				COALESCE(work_orders.refund_tx_hash, work_orders.release_tx_hash) AS chain_tx_hash
			FROM journal_entries
			LEFT JOIN work_orders ON work_orders.id = journal_entries.work_order_id
			WHERE journal_entries.netting_batch_id IS NULL
				AND NULLIF(BTRIM(journal_entries.storage_cid), '') IS NOT NULL
				AND EXISTS (
					SELECT 1
					FROM ledger_entries
					INNER JOIN accounts ON accounts.id = ledger_entries.account_id
					WHERE ledger_entries.journal_entry_id = journal_entries.id
						AND accounts.agent_id = $1
						AND accounts.type IN ('revenue', 'expense')
						AND ($2::timestamptz IS NULL OR ledger_entries.created_at >= $2::timestamptz)
						AND ledger_entries.created_at < $3::timestamptz
				)
		) audit_batches
		ORDER BY audit_batches.anchored_at DESC, audit_batches.batch_id
	`, agentID, pnlTimeArg(request.From), request.To)
	if err != nil {
		return nil, fmt.Errorf("query pnl audit batches: %w", err)
	}
	defer rows.Close()

	batches := []domain.PnLAuditBatch{}
	for rows.Next() {
		var batchID string
		var source string
		var storageRootHash pgtype.Text
		var entryCount int64
		var anchoredAt time.Time
		var chainTxHash pgtype.Text
		if err := rows.Scan(&batchID, &source, &storageRootHash, &entryCount, &anchoredAt, &chainTxHash); err != nil {
			return nil, fmt.Errorf("scan pnl audit batch: %w", err)
		}

		hash := nullableTextPointer(storageRootHash)
		chainHash := nullableTextPointer(chainTxHash)
		batch := domain.PnLAuditBatch{
			BatchID:          batchID,
			Source:           source,
			StorageRootHash:  hash,
			EntryCount:       entryCount,
			AnchoredAt:       anchoredAt.UTC().Format(time.RFC3339),
			ExplorerURL:      pnlExplorerURL(hash),
			ChainExplorerURL: pnlChainExplorerURL(chainHash),
		}
		batches = append(batches, batch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pnl audit batches: %w", err)
	}

	return batches, nil
}

func (r *PnLRepository) fetchPnLTransactions(ctx context.Context, agentID uuid.UUID, request domain.PnLReportRequest) ([]domain.PnLTransaction, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			journal_entries.id::text AS journal_entry_id,
			CASE
				WHEN journal_entries.payment_intent_id IS NOT NULL OR journal_entries.netting_batch_id IS NOT NULL THEN journal_entries.netting_batch_id::text
				ELSE journal_entries.id::text
			END AS audit_batch_id,
			CASE
				WHEN journal_entries.payment_intent_id IS NOT NULL OR journal_entries.netting_batch_id IS NOT NULL THEN 'netting_batch'
				ELSE 'escrow_event'
			END AS source,
			accounts.name AS account_name,
			accounts.type AS account_type,
			ledger_entries.entry_type,
			CASE
				WHEN accounts.type = 'revenue' AND ledger_entries.entry_type = 'credit' THEN ledger_entries.amount
				WHEN accounts.type = 'revenue' AND ledger_entries.entry_type = 'debit' THEN -ledger_entries.amount
				WHEN accounts.type = 'expense' AND ledger_entries.entry_type = 'debit' THEN ledger_entries.amount
				WHEN accounts.type = 'expense' AND ledger_entries.entry_type = 'credit' THEN -ledger_entries.amount
				ELSE 0
			END::text AS amount,
			CASE
				WHEN accounts.type = 'revenue' THEN 'payer'
				WHEN accounts.type = 'expense' THEN 'payee'
				ELSE ''
			END AS counterparty_role,
			CASE
				WHEN accounts.type = 'revenue' THEN COALESCE(payment_payer.wallet_address, work_order_payer.wallet_address, '')
				WHEN accounts.type = 'expense' THEN COALESCE(payment_payee.wallet_address, work_order_payee.wallet_address, '')
				ELSE ''
			END AS counterparty_address,
			CASE
				WHEN journal_entries.payment_intent_id IS NOT NULL THEN 'payment_intent'
				WHEN journal_entries.work_order_id IS NOT NULL THEN 'work_order'
				ELSE ''
			END AS reference_type,
			COALESCE(journal_entries.payment_intent_id::text, journal_entries.work_order_id::text, '') AS reference_id,
			journal_entries.description,
			ledger_entries.created_at AS occurred_at
		FROM ledger_entries
		INNER JOIN accounts ON accounts.id = ledger_entries.account_id
		INNER JOIN journal_entries ON journal_entries.id = ledger_entries.journal_entry_id
		LEFT JOIN work_orders ON work_orders.id = journal_entries.work_order_id
		LEFT JOIN agents work_order_payer ON work_order_payer.id = work_orders.creator_id
		LEFT JOIN agents work_order_payee ON work_order_payee.id = work_orders.provider_id
		LEFT JOIN payment_intents ON payment_intents.id = journal_entries.payment_intent_id
		LEFT JOIN agents payment_payer ON payment_payer.id = payment_intents.payer_id
		LEFT JOIN agents payment_payee ON payment_payee.id = payment_intents.payee_id
		WHERE accounts.agent_id = $1
			AND accounts.type IN ('revenue', 'expense')
			AND ($2::timestamptz IS NULL OR ledger_entries.created_at >= $2::timestamptz)
			AND ledger_entries.created_at < $3::timestamptz
		ORDER BY ledger_entries.created_at DESC, journal_entries.id, accounts.name, ledger_entries.id
	`, agentID, pnlTimeArg(request.From), request.To)
	if err != nil {
		return nil, fmt.Errorf("query pnl transactions: %w", err)
	}
	defer rows.Close()

	transactions := []domain.PnLTransaction{}
	for rows.Next() {
		var journalEntryID string
		var auditBatchID pgtype.Text
		var source string
		var accountName string
		var accountType string
		var entryType string
		var amountText string
		var counterpartyRole string
		var counterpartyAddress string
		var referenceType string
		var referenceID string
		var description string
		var occurredAt time.Time
		if err := rows.Scan(
			&journalEntryID,
			&auditBatchID,
			&source,
			&accountName,
			&accountType,
			&entryType,
			&amountText,
			&counterpartyRole,
			&counterpartyAddress,
			&referenceType,
			&referenceID,
			&description,
			&occurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan pnl transaction: %w", err)
		}

		amount, ok := new(big.Int).SetString(amountText, 10)
		if !ok {
			return nil, fmt.Errorf("invalid pnl transaction amount: %q", amountText)
		}

		transactions = append(transactions, domain.PnLTransaction{
			JournalEntryID: journalEntryID,
			AuditBatchID:   nullableTextPointer(auditBatchID),
			Source:         source,
			Account:        accountName,
			AccountType:    domain.AccountType(accountType),
			EntryType:      domain.LedgerEntryType(entryType),
			Amount:         amount.String(),
			Counterparty: domain.PnLCounterparty{
				Role:    counterpartyRole,
				Address: counterpartyAddress,
			},
			Reference: domain.PnLReference{
				Type: referenceType,
				ID:   referenceID,
			},
			Description: description,
			OccurredAt:  occurredAt.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pnl transactions: %w", err)
	}

	return transactions, nil
}

func pnlTimeArg(value *time.Time) any {
	if value == nil {
		return nil
	}

	return *value
}

func nullableTextPointer(value pgtype.Text) *string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}

	text := value.String
	return &text
}

func pnlExplorerURL(hash *string) *string {
	if hash == nil {
		return nil
	}

	url := pnlExplorerBaseURL + *hash
	return &url
}

func pnlChainExplorerURL(hash *string) *string {
	if hash == nil {
		return nil
	}

	url := pnlChainExplorerBaseURL + *hash
	return &url
}
