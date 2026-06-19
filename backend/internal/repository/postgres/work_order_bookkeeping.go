package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

// This file holds the escrow on-chain event persistence. Each event is a pair:
//
//   - Resolve<Event>Target — a read-only SELECT that matches the event against
//     persisted state and yields the IDs the usecase needs to build the journal
//     anchor before it uploads to 0G. It holds no transaction.
//   - Apply<Event> — a guarded state transition plus the prepared journal/ledger
//     writes, committed in a single transaction with no network I/O held open.
//
// The per-event SQL stays literal (joins/predicates differ); resolveTarget and
// applyTransition factor out the shared execute → scan → wrap boilerplate.

func (r *WorkOrderRepository) ResolveOrderCreatedTarget(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolveTarget(ctx, "resolve order created target", `
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
	)
}

func (r *WorkOrderRepository) ApplyOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	return r.applyTransition(ctx, "apply order created", entry, `
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
}

func (r *WorkOrderRepository) ResolveOrderCreatedRollbackTarget(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolveTarget(ctx, "resolve order created rollback target", `
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
	)
}

func (r *WorkOrderRepository) ApplyOrderCreatedRollback(ctx context.Context, event domain.OrderCreatedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	return r.applyTransition(ctx, "apply order created rollback", entry, `
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
}

func (r *WorkOrderRepository) ResolveOrderReleasedTarget(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolveTarget(ctx, "resolve order released target", `
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
	)
}

func (r *WorkOrderRepository) ApplyOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	return r.applyTransition(ctx, "apply order released", entry, `
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
}

func (r *WorkOrderRepository) ResolveOrderReleasedRollbackTarget(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolveTarget(ctx, "resolve order released rollback target", `
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
	)
}

func (r *WorkOrderRepository) ApplyOrderReleasedRollback(ctx context.Context, event domain.OrderReleasedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	return r.applyTransition(ctx, "apply order released rollback", entry, `
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
}

func (r *WorkOrderRepository) ResolveOrderRefundedTarget(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolveTarget(ctx, "resolve order refunded target", `
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
	)
}

func (r *WorkOrderRepository) ApplyOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	return r.applyTransition(ctx, "apply order refunded", entry, `
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
}

func (r *WorkOrderRepository) ResolveOrderRefundedRollbackTarget(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolveTarget(ctx, "resolve order refunded rollback target", `
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
	)
}

func (r *WorkOrderRepository) ApplyOrderRefundedRollback(ctx context.Context, event domain.OrderRefundedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	return r.applyTransition(ctx, "apply order refunded rollback", entry, `
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
}

// resolveTarget runs a read-only bookkeeping-target SELECT. It never opens a
// transaction, so the 0G upload it precedes holds no DB connection.
func (r *WorkOrderRepository) resolveTarget(ctx context.Context, op string, sql string, args ...any) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	target, matched, err := scanBookkeepingTarget(r.db.QueryRow(ctx, sql, args...))
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", op, err)
	}

	return target, matched, nil
}

// applyTransition runs the guarded state transition and, when it matches a row,
// the prepared journal/ledger writes inside one transaction. The StorageCID was
// produced by an upload that already completed outside this tx.
func (r *WorkOrderRepository) applyTransition(ctx context.Context, op string, entry domain.EscrowJournalEntry, sql string, args ...any) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		matched, err := r.transition(ctx, tx, sql, args...)
		if err != nil || !matched {
			return false, err
		}

		return true, r.persistEscrowJournal(ctx, tx, entry)
	})
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
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
		accountID, err := upsertAccount(ctx, tx, r.nextID, posting.AgentID, posting.AccountName, posting.AccountType, entry.CreatedAt)
		if err != nil {
			return err
		}

		if err := writeLedgerPosting(ctx, tx, r.nextID, accountID, entry.JournalID, posting.AccountType, posting.EntryType, entry.Amount, entry.CreatedAt); err != nil {
			return err
		}
	}

	return nil
}

func (r *WorkOrderRepository) nextID() (uuid.UUID, error) {
	if r.newID != nil {
		return r.newID()
	}

	return uuid.NewV7()
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
