package postgres

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// accountLedgerExec is the minimal transaction surface the double-entry helpers
// need. Both workOrderTx and nettingTx satisfy it, so escrow and netting share a
// single account/ledger persistence path instead of duplicating the SQL.
type accountLedgerExec interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// upsertAccount idempotently resolves the (agent, name) account row, returning the
// stored id. The caller supplies an id generator so each repository keeps its own
// UUID source (uuid.NewV7 or a test stub).
func upsertAccount(
	ctx context.Context,
	exec accountLedgerExec,
	newID func() (uuid.UUID, error),
	agentID uuid.UUID,
	name string,
	accountType domain.AccountType,
	createdAt time.Time,
) (uuid.UUID, error) {
	accountID, err := newID()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate account id: %w", err)
	}

	var storedID uuid.UUID
	err = exec.QueryRow(ctx, `
		INSERT INTO accounts (
			id,
			agent_id,
			name,
			type,
			created_at
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (agent_id, name) DO UPDATE
		SET type = EXCLUDED.type
		RETURNING id
	`,
		accountID,
		agentID,
		name,
		string(accountType),
		createdAt,
	).Scan(&storedID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("find or create account %s: %w", name, err)
	}

	return storedID, nil
}

// writeLedgerPosting inserts one ledger leg and applies its signed balance delta.
func writeLedgerPosting(
	ctx context.Context,
	exec accountLedgerExec,
	newID func() (uuid.UUID, error),
	accountID uuid.UUID,
	journalID uuid.UUID,
	accountType domain.AccountType,
	entryType domain.LedgerEntryType,
	amount big.Int,
	createdAt time.Time,
) error {
	ledgerID, err := newID()
	if err != nil {
		return fmt.Errorf("generate ledger entry id: %w", err)
	}

	_, err = exec.Exec(ctx, `
		INSERT INTO ledger_entries (
			id,
			account_id,
			journal_entry_id,
			amount,
			entry_type,
			created_at
		) VALUES ($1, $2, $3, $4::numeric, $5, $6)
	`,
		ledgerID,
		accountID,
		journalID,
		amount.String(),
		string(entryType),
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("insert ledger entry: %w", err)
	}

	delta := accountBalanceDelta(accountType, entryType, amount)
	_, err = exec.Exec(ctx, `
		UPDATE accounts
		SET
			balance = balance + $1::numeric,
			version = version + 1
		WHERE id = $2
	`,
		delta.String(),
		accountID,
	)
	if err != nil {
		return fmt.Errorf("update account balance: %w", err)
	}

	return nil
}

// accountBalanceDelta returns the signed amount to add to an account's balance for
// a posting, honoring each account type's normal balance side.
func accountBalanceDelta(accountType domain.AccountType, entryType domain.LedgerEntryType, amount big.Int) big.Int {
	delta := *new(big.Int).Set(&amount)
	normalDebit := accountType == domain.AccountTypeAsset || accountType == domain.AccountTypeExpense
	isDebit := entryType == domain.LedgerEntryTypeDebit
	if normalDebit != isDebit {
		delta.Neg(&delta)
	}

	return delta
}
