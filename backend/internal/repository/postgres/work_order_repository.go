package postgres

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workOrderExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type workOrderDB interface {
	workOrderExecutor
}

type workOrderTx interface {
	workOrderExecutor
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type WorkOrderRepository struct {
	db      workOrderDB
	beginTx func(ctx context.Context) (workOrderTx, error)
	newID   func() (uuid.UUID, error)
}

func NewWorkOrderRepository(db *pgxpool.Pool) *WorkOrderRepository {
	return &WorkOrderRepository{
		db: db,
		beginTx: func(ctx context.Context) (workOrderTx, error) {
			tx, err := db.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				return nil, err
			}

			return tx, nil
		},
		newID: uuid.NewV7,
	}
}

func (r *WorkOrderRepository) FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.WorkOrder, error) {
	workOrder, err := scanWorkOrder(r.db.QueryRow(ctx, `
		SELECT
			id,
			idempotency_key,
			creator_id,
			provider_id,
			amount::text,
			status,
			spec_hash,
			spec_version,
			spec_tx_hash,
			deliverable_cid,
			delivered_at,
			completed_at,
			refunded_at,
			onchain_order_id::text,
			order_tx_hash,
			created_at,
			updated_at
		FROM work_orders
		WHERE idempotency_key = $1
	`, idempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find work order by idempotency key: %w", err)
	}

	return workOrder, nil
}

func (r *WorkOrderRepository) FindByOnchainOrderID(ctx context.Context, onchainOrderID big.Int) (*domain.WorkOrder, error) {
	workOrder, err := scanWorkOrder(r.db.QueryRow(ctx, `
		SELECT
			id,
			idempotency_key,
			creator_id,
			provider_id,
			amount::text,
			status,
			spec_hash,
			spec_version,
			spec_tx_hash,
			deliverable_cid,
			delivered_at,
			completed_at,
			refunded_at,
			onchain_order_id::text,
			order_tx_hash,
			created_at,
			updated_at
		FROM work_orders
		WHERE onchain_order_id = $1::numeric
	`, onchainOrderID.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find work order by onchain order id: %w", err)
	}

	return workOrder, nil
}

func (r *WorkOrderRepository) FindDeliveryTargetByOnchainOrderID(ctx context.Context, onchainOrderID big.Int) (*domain.WorkOrderDeliveryTarget, error) {
	var payee string
	workOrder, err := scanWorkOrderWithExtra(r.db.QueryRow(ctx, `
		SELECT
			work_orders.id,
			work_orders.idempotency_key,
			work_orders.creator_id,
			work_orders.provider_id,
			work_orders.amount::text,
			work_orders.status,
			work_orders.spec_hash,
			work_orders.spec_version,
			work_orders.spec_tx_hash,
			work_orders.deliverable_cid,
			work_orders.delivered_at,
			work_orders.completed_at,
			work_orders.refunded_at,
			work_orders.onchain_order_id::text,
			work_orders.order_tx_hash,
			work_orders.created_at,
			work_orders.updated_at,
			payee.wallet_address
		FROM work_orders
		INNER JOIN agents payee ON payee.id = work_orders.provider_id
		WHERE work_orders.onchain_order_id = $1::numeric
	`, onchainOrderID.String()), &payee)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find delivery target by onchain order id: %w", err)
	}

	return &domain.WorkOrderDeliveryTarget{WorkOrder: *workOrder, Payee: payee}, nil
}

func (r *WorkOrderRepository) Create(ctx context.Context, workOrder domain.WorkOrder) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO work_orders (
			id,
			idempotency_key,
			creator_id,
			provider_id,
			amount,
			status,
			spec_hash,
			spec_version,
			spec_tx_hash,
			deliverable_cid,
			delivered_at,
			completed_at,
			refunded_at,
			onchain_order_id,
			order_tx_hash,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5::numeric, $6, $7, $8, $9, $10, $11, $12, $13, $14::numeric, $15, $16, $17
		)
	`,
		workOrder.ID,
		workOrder.IdempotencyKey,
		workOrder.CreatorID,
		workOrder.ProviderID,
		workOrder.Amount.String(),
		string(workOrder.Status),
		workOrder.SpecHash,
		workOrder.SpecVersion,
		workOrder.SpecTxHash,
		stringValue(workOrder.DeliverableCID),
		timeValue(workOrder.DeliveredAt),
		timeValue(workOrder.CompletedAt),
		timeValue(workOrder.RefundedAt),
		bigIntValue(workOrder.OnchainOrderID),
		stringValue(workOrder.OrderTxHash),
		workOrder.CreatedAt,
		workOrder.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert work order: %w", err)
	}

	return nil
}

func (r *WorkOrderRepository) SubmitDelivery(ctx context.Context, update domain.WorkOrderDeliveryUpdate) (bool, error) {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE work_orders
		SET
			deliverable_cid = $1,
			delivered_at = $2,
			updated_at = $2
		WHERE onchain_order_id = $3::numeric
			AND status = $4
			AND deliverable_cid IS NULL
	`,
		update.DeliveryHash,
		update.DeliveredAt,
		update.OnchainOrderID.String(),
		string(domain.WorkOrderStatusFunded),
	)
	if err != nil {
		return false, fmt.Errorf("submit delivery: %w", err)
	}

	return commandTag.RowsAffected() > 0, nil
}

// ResolveOrderCreatedTarget matches an OrderCreated event against a draft work order
// and its counterparties, without mutating any row. It is read-only and holds no
// transaction, so the journal upload it precedes never blocks a DB connection.
func (r *WorkOrderRepository) ResolveOrderCreatedTarget(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	target, matched, err := scanBookkeepingTarget(r.db.QueryRow(ctx, `
		SELECT work_orders.id, work_orders.creator_id, work_orders.provider_id
		FROM work_orders
		INNER JOIN agents payer ON payer.id = work_orders.creator_id
		INNER JOIN agents payee ON payee.id = work_orders.provider_id
		WHERE work_orders.spec_hash = $1
			AND work_orders.amount = $2::numeric
			AND work_orders.status = $3
			AND LOWER(payer.wallet_address) = LOWER($4)
			AND LOWER(payee.wallet_address) = LOWER($5)
	`,
		event.SpecHash,
		event.Amount.String(),
		string(domain.WorkOrderStatusDraft),
		event.Payer,
		event.Payee,
	))
	if err != nil {
		return nil, false, fmt.Errorf("resolve order created target: %w", err)
	}

	return target, matched, nil
}

func (r *WorkOrderRepository) ApplyOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		matched, err := r.transition(ctx, tx, `
			UPDATE work_orders
			SET
				status = $1,
				onchain_order_id = $2::numeric,
				order_tx_hash = $3,
				updated_at = $4
			WHERE id = $5
				AND status = $6
		`,
			string(domain.WorkOrderStatusFunded),
			event.OnchainOrderID.String(),
			event.TransactionHash,
			event.RecordedAt,
			entry.WorkOrderID,
			string(domain.WorkOrderStatusDraft),
		)
		if err != nil || !matched {
			return false, err
		}

		return true, r.persistEscrowJournal(ctx, tx, entry)
	})
	if err != nil {
		return false, fmt.Errorf("apply order created: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) ResolveOrderCreatedRollbackTarget(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	target, matched, err := scanBookkeepingTarget(r.db.QueryRow(ctx, `
		SELECT work_orders.id, work_orders.creator_id, work_orders.provider_id
		FROM work_orders
		INNER JOIN agents payer ON payer.id = work_orders.creator_id
		INNER JOIN agents payee ON payee.id = work_orders.provider_id
		WHERE work_orders.spec_hash = $1
			AND work_orders.amount = $2::numeric
			AND work_orders.onchain_order_id = $3::numeric
			AND work_orders.order_tx_hash = $4
			AND work_orders.status = $5
			AND LOWER(payer.wallet_address) = LOWER($6)
			AND LOWER(payee.wallet_address) = LOWER($7)
	`,
		event.SpecHash,
		event.Amount.String(),
		event.OnchainOrderID.String(),
		event.TransactionHash,
		string(domain.WorkOrderStatusFunded),
		event.Payer,
		event.Payee,
	))
	if err != nil {
		return nil, false, fmt.Errorf("resolve order created rollback target: %w", err)
	}

	return target, matched, nil
}

func (r *WorkOrderRepository) ApplyOrderCreatedRollback(ctx context.Context, event domain.OrderCreatedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		matched, err := r.transition(ctx, tx, `
			UPDATE work_orders
			SET
				status = $1,
				onchain_order_id = NULL,
				order_tx_hash = NULL,
				updated_at = $2
			WHERE id = $3
				AND status = $4
		`,
			string(domain.WorkOrderStatusDraft),
			event.RolledBackAt,
			entry.WorkOrderID,
			string(domain.WorkOrderStatusFunded),
		)
		if err != nil || !matched {
			return false, err
		}

		return true, r.persistEscrowJournal(ctx, tx, entry)
	})
	if err != nil {
		return false, fmt.Errorf("apply order created rollback: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) ResolveOrderReleasedTarget(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	target, matched, err := scanBookkeepingTarget(r.db.QueryRow(ctx, `
		SELECT work_orders.id, work_orders.creator_id, work_orders.provider_id
		FROM work_orders
		INNER JOIN agents payee ON payee.id = work_orders.provider_id
		WHERE work_orders.onchain_order_id = $1::numeric
			AND work_orders.amount = $2::numeric
			AND work_orders.status = $3
			AND LOWER(payee.wallet_address) = LOWER($4)
	`,
		event.OnchainOrderID.String(),
		event.Amount.String(),
		string(domain.WorkOrderStatusFunded),
		event.Payee,
	))
	if err != nil {
		return nil, false, fmt.Errorf("resolve order released target: %w", err)
	}

	return target, matched, nil
}

func (r *WorkOrderRepository) ApplyOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		matched, err := r.transition(ctx, tx, `
			UPDATE work_orders
			SET
				status = $1,
				completed_at = $2,
				release_tx_hash = $3,
				updated_at = $2
			WHERE id = $4
				AND status = $5
		`,
			string(domain.WorkOrderStatusCompleted),
			event.RecordedAt,
			event.TransactionHash,
			entry.WorkOrderID,
			string(domain.WorkOrderStatusFunded),
		)
		if err != nil || !matched {
			return false, err
		}

		return true, r.persistEscrowJournal(ctx, tx, entry)
	})
	if err != nil {
		return false, fmt.Errorf("apply order released: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) ResolveOrderReleasedRollbackTarget(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	target, matched, err := scanBookkeepingTarget(r.db.QueryRow(ctx, `
		SELECT work_orders.id, work_orders.creator_id, work_orders.provider_id
		FROM work_orders
		INNER JOIN agents payee ON payee.id = work_orders.provider_id
		WHERE work_orders.onchain_order_id = $1::numeric
			AND work_orders.amount = $2::numeric
			AND work_orders.status = $3
			AND LOWER(payee.wallet_address) = LOWER($4)
	`,
		event.OnchainOrderID.String(),
		event.Amount.String(),
		string(domain.WorkOrderStatusCompleted),
		event.Payee,
	))
	if err != nil {
		return nil, false, fmt.Errorf("resolve order released rollback target: %w", err)
	}

	return target, matched, nil
}

func (r *WorkOrderRepository) ApplyOrderReleasedRollback(ctx context.Context, event domain.OrderReleasedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		matched, err := r.transition(ctx, tx, `
			UPDATE work_orders
			SET
				status = $1,
				completed_at = NULL,
				release_tx_hash = NULL,
				updated_at = $2
			WHERE id = $3
				AND status = $4
		`,
			string(domain.WorkOrderStatusFunded),
			event.RolledBackAt,
			entry.WorkOrderID,
			string(domain.WorkOrderStatusCompleted),
		)
		if err != nil || !matched {
			return false, err
		}

		return true, r.persistEscrowJournal(ctx, tx, entry)
	})
	if err != nil {
		return false, fmt.Errorf("apply order released rollback: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) ResolveOrderRefundedTarget(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	target, matched, err := scanBookkeepingTarget(r.db.QueryRow(ctx, `
		SELECT work_orders.id, work_orders.creator_id, work_orders.provider_id
		FROM work_orders
		INNER JOIN agents payer ON payer.id = work_orders.creator_id
		WHERE work_orders.onchain_order_id = $1::numeric
			AND work_orders.amount = $2::numeric
			AND work_orders.status = $3
			AND LOWER(payer.wallet_address) = LOWER($4)
	`,
		event.OnchainOrderID.String(),
		event.Amount.String(),
		string(domain.WorkOrderStatusFunded),
		event.Payer,
	))
	if err != nil {
		return nil, false, fmt.Errorf("resolve order refunded target: %w", err)
	}

	return target, matched, nil
}

func (r *WorkOrderRepository) ApplyOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		matched, err := r.transition(ctx, tx, `
			UPDATE work_orders
			SET
				status = $1,
				refunded_at = $2,
				refund_tx_hash = $3,
				updated_at = $2
			WHERE id = $4
				AND status = $5
		`,
			string(domain.WorkOrderStatusRefunded),
			event.RecordedAt,
			event.TransactionHash,
			entry.WorkOrderID,
			string(domain.WorkOrderStatusFunded),
		)
		if err != nil || !matched {
			return false, err
		}

		return true, r.persistEscrowJournal(ctx, tx, entry)
	})
	if err != nil {
		return false, fmt.Errorf("apply order refunded: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) ResolveOrderRefundedRollbackTarget(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	target, matched, err := scanBookkeepingTarget(r.db.QueryRow(ctx, `
		SELECT work_orders.id, work_orders.creator_id, work_orders.provider_id
		FROM work_orders
		INNER JOIN agents payer ON payer.id = work_orders.creator_id
		WHERE work_orders.onchain_order_id = $1::numeric
			AND work_orders.amount = $2::numeric
			AND work_orders.status = $3
			AND LOWER(payer.wallet_address) = LOWER($4)
	`,
		event.OnchainOrderID.String(),
		event.Amount.String(),
		string(domain.WorkOrderStatusRefunded),
		event.Payer,
	))
	if err != nil {
		return nil, false, fmt.Errorf("resolve order refunded rollback target: %w", err)
	}

	return target, matched, nil
}

func (r *WorkOrderRepository) ApplyOrderRefundedRollback(ctx context.Context, event domain.OrderRefundedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		matched, err := r.transition(ctx, tx, `
			UPDATE work_orders
			SET
				status = $1,
				refunded_at = NULL,
				refund_tx_hash = NULL,
				updated_at = $2
			WHERE id = $3
				AND status = $4
		`,
			string(domain.WorkOrderStatusFunded),
			event.RolledBackAt,
			entry.WorkOrderID,
			string(domain.WorkOrderStatusRefunded),
		)
		if err != nil || !matched {
			return false, err
		}

		return true, r.persistEscrowJournal(ctx, tx, entry)
	})
	if err != nil {
		return false, fmt.Errorf("apply order refunded rollback: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) withTx(ctx context.Context, fn func(tx workOrderTx) (bool, error)) (updated bool, err error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	updated, err = fn(tx)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}

	return updated, nil
}

func (r *WorkOrderRepository) begin(ctx context.Context) (workOrderTx, error) {
	if r.beginTx != nil {
		return r.beginTx(ctx)
	}

	return nil, errors.New("transaction support is not configured")
}

// transition runs a guarded UPDATE that flips the work order's status. It reports
// whether a row matched; matched=false means another worker already advanced the
// state (or the row vanished), so the caller skips the journal write.
func (r *WorkOrderRepository) transition(ctx context.Context, tx workOrderTx, sql string, args ...any) (bool, error) {
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return false, err
	}

	return tag.RowsAffected() > 0, nil
}

// persistEscrowJournal writes the prepared journal entry and its ledger postings.
// The 0G upload that produced entry.StorageCID already completed outside this tx.
func (r *WorkOrderRepository) persistEscrowJournal(ctx context.Context, tx workOrderTx, entry domain.EscrowJournalEntry) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO journal_entries (
			id,
			idempotency_key,
			work_order_id,
			description,
			storage_cid,
			netting_batch_id,
			created_at
		) VALUES ($1, $2, $3, $4, $5, NULL, $6)
	`,
		entry.JournalID,
		entry.IdempotencyKey,
		entry.WorkOrderID,
		entry.Description,
		entry.StorageCID,
		entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert journal entry: %w", err)
	}

	for _, posting := range entry.Postings {
		accountID, err := r.findOrCreateAccount(ctx, tx, posting, entry.CreatedAt)
		if err != nil {
			return err
		}

		if err := r.recordLedgerPosting(ctx, tx, accountID, entry.JournalID, posting, entry.Amount, entry.CreatedAt); err != nil {
			return err
		}
	}

	return nil
}

func (r *WorkOrderRepository) findOrCreateAccount(ctx context.Context, tx workOrderTx, posting domain.LedgerPosting, createdAt time.Time) (uuid.UUID, error) {
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

func (r *WorkOrderRepository) recordLedgerPosting(
	ctx context.Context,
	tx workOrderTx,
	accountID uuid.UUID,
	journalID uuid.UUID,
	posting domain.LedgerPosting,
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

func (r *WorkOrderRepository) nextID() (uuid.UUID, error) {
	if r.newID != nil {
		return r.newID()
	}

	return uuid.NewV7()
}

func accountBalanceDelta(accountType domain.AccountType, entryType domain.LedgerEntryType, amount big.Int) big.Int {
	delta := *new(big.Int).Set(&amount)
	normalDebit := accountType == domain.AccountTypeAsset || accountType == domain.AccountTypeExpense
	isDebit := entryType == domain.LedgerEntryTypeDebit
	if normalDebit != isDebit {
		delta.Neg(&delta)
	}

	return delta
}

func scanBookkeepingTarget(row pgx.Row) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	var target domain.WorkOrderBookkeepingTarget
	err := row.Scan(&target.WorkOrderID, &target.PayerID, &target.PayeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	return &target, true, nil
}

func scanWorkOrder(row pgx.Row) (*domain.WorkOrder, error) {
	return scanWorkOrderWithExtra(row)
}

func scanWorkOrderWithExtra(row pgx.Row, extraDest ...any) (*domain.WorkOrder, error) {
	var workOrder domain.WorkOrder
	var amountText string
	var status string
	var deliverableCID pgtype.Text
	var deliveredAt pgtype.Timestamptz
	var completedAt pgtype.Timestamptz
	var refundedAt pgtype.Timestamptz
	var onchainOrderIDText pgtype.Text
	var orderTxHash pgtype.Text

	dest := []any{
		&workOrder.ID,
		&workOrder.IdempotencyKey,
		&workOrder.CreatorID,
		&workOrder.ProviderID,
		&amountText,
		&status,
		&workOrder.SpecHash,
		&workOrder.SpecVersion,
		&workOrder.SpecTxHash,
		&deliverableCID,
		&deliveredAt,
		&completedAt,
		&refundedAt,
		&onchainOrderIDText,
		&orderTxHash,
		&workOrder.CreatedAt,
		&workOrder.UpdatedAt,
	}
	dest = append(dest, extraDest...)

	if err := row.Scan(dest...); err != nil {
		return nil, err
	}

	amount, ok := new(big.Int).SetString(amountText, 10)
	if !ok {
		return nil, fmt.Errorf("invalid work order amount: %q", amountText)
	}
	workOrder.Amount = *amount
	workOrder.Status = domain.WorkOrderStatus(status)

	if deliverableCID.Valid {
		value := deliverableCID.String
		workOrder.DeliverableCID = &value
	}
	if deliveredAt.Valid {
		value := deliveredAt.Time
		workOrder.DeliveredAt = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		workOrder.CompletedAt = &value
	}
	if refundedAt.Valid {
		value := refundedAt.Time
		workOrder.RefundedAt = &value
	}
	if onchainOrderIDText.Valid {
		value, ok := new(big.Int).SetString(onchainOrderIDText.String, 10)
		if !ok {
			return nil, fmt.Errorf("invalid work order onchain order id: %q", onchainOrderIDText.String)
		}
		workOrder.OnchainOrderID = value
	}
	if orderTxHash.Valid {
		value := orderTxHash.String
		workOrder.OrderTxHash = &value
	}

	return &workOrder, nil
}

func stringValue(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}

func timeValue(value *time.Time) any {
	if value == nil {
		return nil
	}

	return *value
}

func bigIntValue(value *big.Int) any {
	if value == nil {
		return nil
	}

	return value.String()
}
