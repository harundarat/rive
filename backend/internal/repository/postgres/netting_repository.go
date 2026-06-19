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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	accountNameCash              = "Cash"
	accountNameNettingPayable    = "Netting Payable"
	accountNameNettingReceivable = "Netting Receivable"
)

// ledgerPosting is one leg of a netting journal entry. Unlike escrow postings
// (which share a single amount and live in domain.LedgerPosting), each netting
// posting carries its own amount, so this type keeps a per-posting Amount.
type ledgerPosting struct {
	AgentID     uuid.UUID
	AccountName string
	AccountType domain.AccountType
	EntryType   domain.LedgerEntryType
	Amount      big.Int
}

type nettingExecutor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type nettingDB interface {
	nettingExecutor
}

type nettingTx interface {
	nettingExecutor
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type NettingRepository struct {
	db      nettingDB
	beginTx func(ctx context.Context) (nettingTx, error)
	newID   func() (uuid.UUID, error)
}

func NewNettingRepository(db *pgxpool.Pool) *NettingRepository {
	return &NettingRepository{
		db: db,
		beginTx: func(ctx context.Context) (nettingTx, error) {
			tx, err := db.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				return nil, err
			}

			return tx, nil
		},
		newID: uuid.NewV7,
	}
}

func (r *NettingRepository) FindIntentByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.PaymentIntent, error) {
	intent, err := scanPaymentIntent(r.db.QueryRow(ctx, `
		SELECT
			payment_intents.id,
			payment_intents.idempotency_key,
			payment_intents.payer_id,
			payment_intents.payee_id,
			payer.wallet_address,
			payee.wallet_address,
			payment_intents.amount::text,
			payment_intents.asset,
			payment_intents.status,
			payment_intents.netting_batch_id,
			payment_intents.failure_reason,
			payment_intents.created_at,
			payment_intents.updated_at,
			payment_intents.settled_at
		FROM payment_intents
		INNER JOIN agents payer ON payer.id = payment_intents.payer_id
		INNER JOIN agents payee ON payee.id = payment_intents.payee_id
		WHERE payment_intents.idempotency_key = $1
	`, idempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find payment intent by idempotency key: %w", err)
	}

	return intent, nil
}

func (r *NettingRepository) CreateIntentWithAccrual(ctx context.Context, intent domain.PaymentIntent) (*domain.PaymentIntent, error) {
	err := r.withTx(ctx, func(tx nettingTx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO payment_intents (
				id,
				idempotency_key,
				payer_id,
				payee_id,
				amount,
				asset,
				status,
				netting_batch_id,
				failure_reason,
				created_at,
				updated_at,
				settled_at
			) VALUES (
				$1, $2, $3, $4, $5::numeric, $6, $7, NULL, NULL, $8, $9, NULL
			)
		`,
			intent.ID,
			intent.IdempotencyKey,
			intent.PayerID,
			intent.PayeeID,
			intent.Amount.String(),
			intent.Asset,
			string(intent.Status),
			intent.CreatedAt,
			intent.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert payment intent: %w", err)
		}

		return r.recordPaymentIntentAccrual(ctx, tx, intent)
	})
	if err != nil {
		return nil, err
	}

	return r.FindIntentByIdempotencyKey(ctx, intent.IdempotencyKey)
}

func (r *NettingRepository) ClaimPendingIntents(ctx context.Context, windowEnd time.Time, batchID uuid.UUID, claimedAt time.Time) (*domain.NettingBatchClaim, error) {
	claim := &domain.NettingBatchClaim{Intents: []domain.PaymentIntent{}}
	err := r.withTx(ctx, func(tx nettingTx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				payment_intents.id,
				payment_intents.idempotency_key,
				payment_intents.payer_id,
				payment_intents.payee_id,
				payer.wallet_address,
				payee.wallet_address,
				payment_intents.amount::text,
				payment_intents.asset,
				payment_intents.status,
				payment_intents.netting_batch_id,
				payment_intents.failure_reason,
				payment_intents.created_at,
				payment_intents.updated_at,
				payment_intents.settled_at
			FROM payment_intents
			INNER JOIN agents payer ON payer.id = payment_intents.payer_id
			INNER JOIN agents payee ON payee.id = payment_intents.payee_id
			WHERE payment_intents.status = $1
				AND payment_intents.created_at <= $2
			ORDER BY payment_intents.created_at, payment_intents.id
			FOR UPDATE OF payment_intents SKIP LOCKED
		`, string(domain.PaymentIntentStatusPending), windowEnd)
		if err != nil {
			return fmt.Errorf("select pending payment intents: %w", err)
		}
		defer rows.Close()

		var ids []uuid.UUID
		var windowStart *time.Time
		for rows.Next() {
			intent, err := scanPaymentIntentFromRows(rows)
			if err != nil {
				return err
			}

			claim.Intents = append(claim.Intents, *intent)
			ids = append(ids, intent.ID)
			if windowStart == nil || intent.CreatedAt.Before(*windowStart) {
				value := intent.CreatedAt
				windowStart = &value
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate pending payment intents: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO netting_batches (
				id,
				batch_status,
				created_at,
				updated_at,
				window_start,
				window_end
			) VALUES ($1, $2, $3, $4, $5, $6)
		`,
			batchID,
			string(domain.BatchStatusProcessing),
			claimedAt,
			claimedAt,
			timeValue(windowStart),
			windowEnd,
		)
		if err != nil {
			return fmt.Errorf("insert netting batch: %w", err)
		}

		_, err = tx.Exec(ctx, `
			UPDATE payment_intents
			SET
				status = $1,
				netting_batch_id = $2,
				updated_at = $3
			WHERE id = ANY($4::uuid[])
		`, string(domain.PaymentIntentStatusBatched), batchID, claimedAt, ids)
		if err != nil {
			return fmt.Errorf("assign payment intents to netting batch: %w", err)
		}

		_, err = tx.Exec(ctx, `
			UPDATE journal_entries
			SET netting_batch_id = $1
			WHERE payment_intent_id = ANY($2::uuid[])
		`, batchID, ids)
		if err != nil {
			return fmt.Errorf("assign payment intent journals to netting batch: %w", err)
		}

		claim.Batch = domain.NettingBatch{
			ID:          batchID,
			BatchStatus: domain.BatchStatusProcessing,
			CreatedAt:   claimedAt,
			UpdatedAt:   claimedAt,
			WindowStart: windowStart,
			WindowEnd:   &windowEnd,
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return claim, nil
}

func (r *NettingRepository) MarkBatchSettled(ctx context.Context, settlement domain.NettingBatchSettlement) error {
	return r.withTx(ctx, func(tx nettingTx) error {
		_, err := tx.Exec(ctx, `
			UPDATE netting_batches
			SET
				batch_status = $1,
				batch_hash = $2,
				manifest_tx_hash = $3,
				settlement_tx_hash = $4,
				gross_intent_count = $5,
				settlement_transfer_count = $6,
				gross_amount = $7::numeric,
				net_amount = $8::numeric,
				updated_at = $9
			WHERE id = $10
				AND batch_status = $11
		`,
			string(domain.BatchStatusSettled),
			settlement.BatchHash,
			settlement.ManifestTxHash,
			stringValue(settlement.SettlementTxHash),
			settlement.GrossIntentCount,
			settlement.SettlementTransferCount,
			settlement.GrossAmount.String(),
			settlement.NetAmount.String(),
			settlement.SettledAt,
			settlement.BatchID,
			string(domain.BatchStatusProcessing),
		)
		if err != nil {
			return fmt.Errorf("mark netting batch settled: %w", err)
		}

		_, err = tx.Exec(ctx, `
			UPDATE payment_intents
			SET
				status = $1,
				updated_at = $2,
				settled_at = $2
			WHERE netting_batch_id = $3
				AND status = $4
		`, string(domain.PaymentIntentStatusSettled), settlement.SettledAt, settlement.BatchID, string(domain.PaymentIntentStatusBatched))
		if err != nil {
			return fmt.Errorf("mark payment intents settled: %w", err)
		}

		return r.recordBatchSettlementBookkeeping(ctx, tx, settlement)
	})
}

func (r *NettingRepository) MarkBatchFailed(ctx context.Context, batchID uuid.UUID, reason string, failedAt time.Time) error {
	reason = truncateFailureReason(reason)
	return r.withTx(ctx, func(tx nettingTx) error {
		_, err := tx.Exec(ctx, `
			UPDATE netting_batches
			SET
				batch_status = $1,
				failure_reason = $2,
				updated_at = $3
			WHERE id = $4
		`, string(domain.BatchStatusFailed), reason, failedAt, batchID)
		if err != nil {
			return fmt.Errorf("mark netting batch failed: %w", err)
		}

		_, err = tx.Exec(ctx, `
			UPDATE payment_intents
			SET
				status = $1,
				failure_reason = $2,
				updated_at = $3
			WHERE netting_batch_id = $4
				AND status = $5
		`, string(domain.PaymentIntentStatusFailed), reason, failedAt, batchID, string(domain.PaymentIntentStatusBatched))
		if err != nil {
			return fmt.Errorf("mark payment intents failed: %w", err)
		}

		return nil
	})
}

func (r *NettingRepository) recordPaymentIntentAccrual(ctx context.Context, tx nettingTx, intent domain.PaymentIntent) error {
	return r.recordNettingJournal(ctx, tx, nettingJournal{
		IdempotencyKey:  fmt.Sprintf("payment_intent:%s:accrual", intent.ID),
		PaymentIntentID: &intent.ID,
		Description:     fmt.Sprintf("Netting payment intent %s accrual", intent.ID),
		CreatedAt:       intent.CreatedAt,
		Postings: []ledgerPosting{
			{
				AgentID:     intent.PayerID,
				AccountName: domain.AccountNameServiceExpense,
				AccountType: domain.AccountTypeExpense,
				EntryType:   domain.LedgerEntryTypeDebit,
				Amount:      intent.Amount,
			},
			{
				AgentID:     intent.PayerID,
				AccountName: accountNameNettingPayable,
				AccountType: domain.AccountTypeLiability,
				EntryType:   domain.LedgerEntryTypeCredit,
				Amount:      intent.Amount,
			},
			{
				AgentID:     intent.PayeeID,
				AccountName: accountNameNettingReceivable,
				AccountType: domain.AccountTypeAsset,
				EntryType:   domain.LedgerEntryTypeDebit,
				Amount:      intent.Amount,
			},
			{
				AgentID:     intent.PayeeID,
				AccountName: domain.AccountNameServiceRevenue,
				AccountType: domain.AccountTypeRevenue,
				EntryType:   domain.LedgerEntryTypeCredit,
				Amount:      intent.Amount,
			},
		},
	})
}

func (r *NettingRepository) recordBatchSettlementBookkeeping(ctx context.Context, tx nettingTx, settlement domain.NettingBatchSettlement) error {
	gross, err := r.fetchBatchGrossPositions(ctx, tx, settlement.BatchID)
	if err != nil {
		return err
	}

	entry := nettingJournal{
		IdempotencyKey: fmt.Sprintf("netting_batch:%s:settlement", settlement.BatchID),
		NettingBatchID: &settlement.BatchID,
		Description:    fmt.Sprintf("Netting batch %s settlement", settlement.BatchID),
		CreatedAt:      settlement.SettledAt,
		Postings:       []ledgerPosting{},
	}

	for agentID, position := range gross {
		if position.Payable.Sign() > 0 {
			entry.Postings = append(entry.Postings, ledgerPosting{
				AgentID:     agentID,
				AccountName: accountNameNettingPayable,
				AccountType: domain.AccountTypeLiability,
				EntryType:   domain.LedgerEntryTypeDebit,
				Amount:      position.Payable,
			})
		}
		if position.Receivable.Sign() > 0 {
			entry.Postings = append(entry.Postings, ledgerPosting{
				AgentID:     agentID,
				AccountName: accountNameNettingReceivable,
				AccountType: domain.AccountTypeAsset,
				EntryType:   domain.LedgerEntryTypeCredit,
				Amount:      position.Receivable,
			})
		}

		net := new(big.Int).Sub(&position.Receivable, &position.Payable)
		if net.Sign() > 0 {
			entry.Postings = append(entry.Postings, ledgerPosting{
				AgentID:     agentID,
				AccountName: accountNameCash,
				AccountType: domain.AccountTypeAsset,
				EntryType:   domain.LedgerEntryTypeDebit,
				Amount:      *net,
			})
		}
		if net.Sign() < 0 {
			net.Abs(net)
			entry.Postings = append(entry.Postings, ledgerPosting{
				AgentID:     agentID,
				AccountName: accountNameCash,
				AccountType: domain.AccountTypeAsset,
				EntryType:   domain.LedgerEntryTypeCredit,
				Amount:      *net,
			})
		}
	}

	return r.recordNettingJournal(ctx, tx, entry)
}

type grossPosition struct {
	Payable    big.Int
	Receivable big.Int
}

func (r *NettingRepository) fetchBatchGrossPositions(ctx context.Context, tx nettingTx, batchID uuid.UUID) (map[uuid.UUID]grossPosition, error) {
	rows, err := tx.Query(ctx, `
		SELECT agent_id, SUM(payable)::text, SUM(receivable)::text
		FROM (
			SELECT payer_id AS agent_id, amount AS payable, 0::numeric AS receivable
			FROM payment_intents
			WHERE netting_batch_id = $1
			UNION ALL
			SELECT payee_id AS agent_id, 0::numeric AS payable, amount AS receivable
			FROM payment_intents
			WHERE netting_batch_id = $1
		) positions
		GROUP BY agent_id
	`, batchID)
	if err != nil {
		return nil, fmt.Errorf("fetch netting batch gross positions: %w", err)
	}
	defer rows.Close()

	positions := map[uuid.UUID]grossPosition{}
	for rows.Next() {
		var agentID uuid.UUID
		var payableText string
		var receivableText string
		if err := rows.Scan(&agentID, &payableText, &receivableText); err != nil {
			return nil, fmt.Errorf("scan netting gross position: %w", err)
		}

		payable, ok := new(big.Int).SetString(payableText, 10)
		if !ok {
			return nil, fmt.Errorf("invalid payable amount: %q", payableText)
		}
		receivable, ok := new(big.Int).SetString(receivableText, 10)
		if !ok {
			return nil, fmt.Errorf("invalid receivable amount: %q", receivableText)
		}

		positions[agentID] = grossPosition{Payable: *payable, Receivable: *receivable}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate netting gross positions: %w", err)
	}

	return positions, nil
}

type nettingJournal struct {
	IdempotencyKey  string
	PaymentIntentID *uuid.UUID
	NettingBatchID  *uuid.UUID
	Description     string
	CreatedAt       time.Time
	Postings        []ledgerPosting
}

func (r *NettingRepository) recordNettingJournal(ctx context.Context, tx nettingTx, entry nettingJournal) error {
	journalID, err := r.nextID()
	if err != nil {
		return fmt.Errorf("generate journal entry id: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO journal_entries (
			id,
			idempotency_key,
			work_order_id,
			payment_intent_id,
			description,
			storage_cid,
			netting_batch_id,
			created_at
		) VALUES ($1, $2, NULL, $3, $4, NULL, $5, $6)
	`,
		journalID,
		entry.IdempotencyKey,
		uuidValue(entry.PaymentIntentID),
		entry.Description,
		uuidValue(entry.NettingBatchID),
		entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert netting journal entry: %w", err)
	}

	for _, posting := range entry.Postings {
		if posting.Amount.Sign() <= 0 {
			continue
		}

		accountID, err := r.findOrCreateAccount(ctx, tx, posting, entry.CreatedAt)
		if err != nil {
			return err
		}

		if err := r.recordLedgerPosting(ctx, tx, accountID, journalID, posting, posting.Amount, entry.CreatedAt); err != nil {
			return err
		}
	}

	return nil
}

func (r *NettingRepository) findOrCreateAccount(ctx context.Context, tx nettingTx, posting ledgerPosting, createdAt time.Time) (uuid.UUID, error) {
	accountID, err := r.nextID()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate account id: %w", err)
	}

	var storedID uuid.UUID
	err = tx.QueryRow(ctx, `
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
		posting.AgentID,
		posting.AccountName,
		string(posting.AccountType),
		createdAt,
	).Scan(&storedID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("find or create account %s: %w", posting.AccountName, err)
	}

	return storedID, nil
}

func (r *NettingRepository) recordLedgerPosting(
	ctx context.Context,
	tx nettingTx,
	accountID uuid.UUID,
	journalID uuid.UUID,
	posting ledgerPosting,
	amount big.Int,
	createdAt time.Time,
) error {
	ledgerID, err := r.nextID()
	if err != nil {
		return fmt.Errorf("generate ledger entry id: %w", err)
	}

	_, err = tx.Exec(ctx, `
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
		string(posting.EntryType),
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("insert ledger entry: %w", err)
	}

	delta := accountBalanceDelta(posting.AccountType, posting.EntryType, amount)
	_, err = tx.Exec(ctx, `
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

func (r *NettingRepository) withTx(ctx context.Context, fn func(tx nettingTx) error) (err error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (r *NettingRepository) begin(ctx context.Context) (nettingTx, error) {
	if r.beginTx != nil {
		return r.beginTx(ctx)
	}

	return nil, errors.New("transaction support is not configured")
}

func (r *NettingRepository) nextID() (uuid.UUID, error) {
	if r.newID != nil {
		return r.newID()
	}

	return uuid.NewV7()
}

func scanPaymentIntent(row pgx.Row) (*domain.PaymentIntent, error) {
	var intent domain.PaymentIntent
	var amountText string
	var status string
	var batchID pgtype.UUID
	var failureReason pgtype.Text
	var settledAt pgtype.Timestamptz
	if err := row.Scan(
		&intent.ID,
		&intent.IdempotencyKey,
		&intent.PayerID,
		&intent.PayeeID,
		&intent.PayerWallet,
		&intent.PayeeWallet,
		&amountText,
		&intent.Asset,
		&status,
		&batchID,
		&failureReason,
		&intent.CreatedAt,
		&intent.UpdatedAt,
		&settledAt,
	); err != nil {
		return nil, err
	}

	return finishScanPaymentIntent(intent, amountText, status, batchID, failureReason, settledAt)
}

func scanPaymentIntentFromRows(rows pgx.Rows) (*domain.PaymentIntent, error) {
	var intent domain.PaymentIntent
	var amountText string
	var status string
	var batchID pgtype.UUID
	var failureReason pgtype.Text
	var settledAt pgtype.Timestamptz
	if err := rows.Scan(
		&intent.ID,
		&intent.IdempotencyKey,
		&intent.PayerID,
		&intent.PayeeID,
		&intent.PayerWallet,
		&intent.PayeeWallet,
		&amountText,
		&intent.Asset,
		&status,
		&batchID,
		&failureReason,
		&intent.CreatedAt,
		&intent.UpdatedAt,
		&settledAt,
	); err != nil {
		return nil, fmt.Errorf("scan payment intent: %w", err)
	}

	return finishScanPaymentIntent(intent, amountText, status, batchID, failureReason, settledAt)
}

func finishScanPaymentIntent(
	intent domain.PaymentIntent,
	amountText string,
	status string,
	batchID pgtype.UUID,
	failureReason pgtype.Text,
	settledAt pgtype.Timestamptz,
) (*domain.PaymentIntent, error) {
	amount, ok := new(big.Int).SetString(amountText, 10)
	if !ok {
		return nil, fmt.Errorf("invalid payment intent amount: %q", amountText)
	}
	intent.Amount = *amount
	intent.Status = domain.PaymentIntentStatus(status)

	if batchID.Valid {
		value := uuid.UUID(batchID.Bytes)
		intent.NettingBatchID = &value
	}
	if failureReason.Valid {
		value := failureReason.String
		intent.FailureReason = &value
	}
	if settledAt.Valid {
		value := settledAt.Time
		intent.SettledAt = &value
	}

	return &intent, nil
}

func uuidValue(value *uuid.UUID) any {
	if value == nil {
		return nil
	}

	return *value
}

func truncateFailureReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 1000 {
		return reason[:1000]
	}

	return reason
}
