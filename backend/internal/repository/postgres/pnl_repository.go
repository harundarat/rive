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

const pnlExplorerBaseURL = "https://storagescan.0g.ai/tx/"

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

	netIncome := new(big.Int).Sub(totalRevenue, totalExpenses)

	return &domain.PnLReport{
		Agent: domain.PnLAgent{
			Address:      agentAddress,
			RegisteredAt: registeredAt.UTC().Format(time.RFC3339),
		},
		Asset: domain.PnLAssetRUSD,
		Summary: domain.PnLSummary{
			TotalRevenue:     totalRevenue.String(),
			TotalExpenses:    totalExpenses.String(),
			NetIncome:        netIncome.String(),
			TransactionCount: transactionCount,
		},
		Revenue:  revenue,
		Expenses: expenses,
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
			netting_batches.id::text,
			COALESCE(MIN(NULLIF(journal_entries.storage_cid, '')), netting_batches.manifest_cid, netting_batches.settlement_tx_hash) AS storage_root_hash,
			COUNT(DISTINCT journal_entries.id)::bigint AS entry_count,
			netting_batches.updated_at
		FROM ledger_entries
		INNER JOIN accounts ON accounts.id = ledger_entries.account_id
		INNER JOIN journal_entries ON journal_entries.id = ledger_entries.journal_entry_id
		INNER JOIN netting_batches ON netting_batches.id = journal_entries.netting_batch_id
		WHERE accounts.agent_id = $1
			AND accounts.type IN ('revenue', 'expense')
			AND ($2::timestamptz IS NULL OR ledger_entries.created_at >= $2::timestamptz)
			AND ledger_entries.created_at < $3::timestamptz
		GROUP BY netting_batches.id, netting_batches.manifest_cid, netting_batches.settlement_tx_hash, netting_batches.updated_at
		ORDER BY netting_batches.updated_at DESC, netting_batches.id
	`, agentID, pnlTimeArg(request.From), request.To)
	if err != nil {
		return nil, fmt.Errorf("query pnl audit batches: %w", err)
	}
	defer rows.Close()

	batches := []domain.PnLAuditBatch{}
	for rows.Next() {
		var batchID string
		var storageRootHash pgtype.Text
		var entryCount int64
		var anchoredAt time.Time
		if err := rows.Scan(&batchID, &storageRootHash, &entryCount, &anchoredAt); err != nil {
			return nil, fmt.Errorf("scan pnl audit batch: %w", err)
		}

		hash := nullableTextPointer(storageRootHash)
		batch := domain.PnLAuditBatch{
			BatchID:         batchID,
			StorageRootHash: hash,
			EntryCount:      entryCount,
			AnchoredAt:      anchoredAt.UTC().Format(time.RFC3339),
			ExplorerURL:     pnlExplorerURL(hash),
		}
		batches = append(batches, batch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pnl audit batches: %w", err)
	}

	return batches, nil
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
