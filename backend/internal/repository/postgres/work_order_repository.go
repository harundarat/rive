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
	accountNameEscrowLocked   = "escrow_locked"
	accountNameEscrowPending  = "escrow_pending"
	accountNameServiceExpense = "Service Expense"
	accountNameServiceRevenue = "Service Revenue"
)

type workOrderExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type workOrderDB interface {
	workOrderExecutor
}

type workOrderStorageUploader interface {
	UploadJSON(ctx context.Context, data any) (*domain.ZGUploadOutput, error)
}

type workOrderTx interface {
	workOrderExecutor
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type WorkOrderRepository struct {
	db              workOrderDB
	storageUploader workOrderStorageUploader
	beginTx         func(ctx context.Context) (workOrderTx, error)
	newID           func() (uuid.UUID, error)
}

func NewWorkOrderRepository(db *pgxpool.Pool, storageUploader workOrderStorageUploader) *WorkOrderRepository {
	return &WorkOrderRepository{
		db:              db,
		storageUploader: storageUploader,
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

func (r *WorkOrderRepository) RecordOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		target, err := scanWorkOrderBookkeepingTarget(tx.QueryRow(ctx, `
			UPDATE work_orders
			SET
				status = $1,
				onchain_order_id = $3::numeric,
				order_tx_hash = $4,
				updated_at = $2
			FROM agents payer, agents payee
			WHERE work_orders.spec_hash = $5
				AND work_orders.amount = $6::numeric
				AND work_orders.status = $7
				AND payer.id = work_orders.creator_id
				AND payee.id = work_orders.provider_id
				AND LOWER(payer.wallet_address) = LOWER($8)
				AND LOWER(payee.wallet_address) = LOWER($9)
			RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id
		`,
			string(domain.WorkOrderStatusFunded),
			event.RecordedAt,
			event.OnchainOrderID.String(),
			event.TransactionHash,
			event.SpecHash,
			event.Amount.String(),
			string(domain.WorkOrderStatusDraft),
			event.Payer,
			event.Payee,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		err = r.recordBookkeeping(ctx, tx, bookkeepingEntry{
			WorkOrderID:    target.WorkOrderID,
			CreatedAt:      event.RecordedAt,
			Amount:         event.Amount,
			IdempotencyKey: escrowJournalKey(target.WorkOrderID, "order_created", event.TransactionHash, event.BlockNumber, event.LogIndex, event.OnchainOrderID),
			Description:    fmt.Sprintf("Escrow order created for on-chain order %s", event.OnchainOrderID.String()),
			Postings: []ledgerPosting{
				{
					AgentID:     target.PayerID,
					AccountName: accountNameEscrowLocked,
					AccountType: domain.AccountTypeAsset,
					EntryType:   domain.LedgerEntryTypeDebit,
				},
				{
					AgentID:     target.PayeeID,
					AccountName: accountNameEscrowPending,
					AccountType: domain.AccountTypeLiability,
					EntryType:   domain.LedgerEntryTypeCredit,
				},
			},
		})
		if err != nil {
			return false, err
		}

		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("record order created: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) RollbackOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		target, err := scanWorkOrderBookkeepingTarget(tx.QueryRow(ctx, `
			UPDATE work_orders
			SET
				status = $1,
				onchain_order_id = NULL,
				order_tx_hash = NULL,
				updated_at = $2
			FROM agents payer, agents payee
			WHERE work_orders.spec_hash = $3
				AND work_orders.amount = $4::numeric
				AND work_orders.onchain_order_id = $5::numeric
				AND work_orders.order_tx_hash = $6
				AND work_orders.status = $7
				AND payer.id = work_orders.creator_id
				AND payee.id = work_orders.provider_id
				AND LOWER(payer.wallet_address) = LOWER($8)
				AND LOWER(payee.wallet_address) = LOWER($9)
			RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id
		`,
			string(domain.WorkOrderStatusDraft),
			event.RolledBackAt,
			event.SpecHash,
			event.Amount.String(),
			event.OnchainOrderID.String(),
			event.TransactionHash,
			string(domain.WorkOrderStatusFunded),
			event.Payer,
			event.Payee,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		err = r.recordBookkeeping(ctx, tx, bookkeepingEntry{
			WorkOrderID:    target.WorkOrderID,
			CreatedAt:      event.RolledBackAt,
			Amount:         event.Amount,
			IdempotencyKey: escrowJournalKey(target.WorkOrderID, "order_created_rollback", event.TransactionHash, event.BlockNumber, event.LogIndex, event.OnchainOrderID),
			Description:    fmt.Sprintf("Escrow order created rollback for on-chain order %s", event.OnchainOrderID.String()),
			Postings: []ledgerPosting{
				{
					AgentID:     target.PayeeID,
					AccountName: accountNameEscrowPending,
					AccountType: domain.AccountTypeLiability,
					EntryType:   domain.LedgerEntryTypeDebit,
				},
				{
					AgentID:     target.PayerID,
					AccountName: accountNameEscrowLocked,
					AccountType: domain.AccountTypeAsset,
					EntryType:   domain.LedgerEntryTypeCredit,
				},
			},
		})
		if err != nil {
			return false, err
		}

		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("rollback order created: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) RecordOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		target, err := scanWorkOrderBookkeepingTarget(tx.QueryRow(ctx, `
			UPDATE work_orders
			SET
				status = $1,
				completed_at = $2,
				updated_at = $2
			FROM agents payee
			WHERE work_orders.onchain_order_id = $3::numeric
				AND work_orders.amount = $4::numeric
				AND work_orders.status = $5
				AND payee.id = work_orders.provider_id
				AND LOWER(payee.wallet_address) = LOWER($6)
			RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id
		`,
			string(domain.WorkOrderStatusCompleted),
			event.RecordedAt,
			event.OnchainOrderID.String(),
			event.Amount.String(),
			string(domain.WorkOrderStatusFunded),
			event.Payee,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		err = r.recordBookkeeping(ctx, tx, bookkeepingEntry{
			WorkOrderID:    target.WorkOrderID,
			CreatedAt:      event.RecordedAt,
			Amount:         event.Amount,
			IdempotencyKey: escrowJournalKey(target.WorkOrderID, "order_released", event.TransactionHash, event.BlockNumber, event.LogIndex, event.OnchainOrderID),
			Description:    fmt.Sprintf("Escrow order released for on-chain order %s", event.OnchainOrderID.String()),
			Postings: []ledgerPosting{
				{
					AgentID:     target.PayeeID,
					AccountName: accountNameEscrowPending,
					AccountType: domain.AccountTypeLiability,
					EntryType:   domain.LedgerEntryTypeDebit,
				},
				{
					AgentID:     target.PayerID,
					AccountName: accountNameEscrowLocked,
					AccountType: domain.AccountTypeAsset,
					EntryType:   domain.LedgerEntryTypeCredit,
				},
				{
					AgentID:     target.PayerID,
					AccountName: accountNameServiceExpense,
					AccountType: domain.AccountTypeExpense,
					EntryType:   domain.LedgerEntryTypeDebit,
				},
				{
					AgentID:     target.PayeeID,
					AccountName: accountNameServiceRevenue,
					AccountType: domain.AccountTypeRevenue,
					EntryType:   domain.LedgerEntryTypeCredit,
				},
			},
		})
		if err != nil {
			return false, err
		}

		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("record order released: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) RollbackOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		target, err := scanWorkOrderBookkeepingTarget(tx.QueryRow(ctx, `
			UPDATE work_orders
			SET
				status = $1,
				completed_at = NULL,
				updated_at = $2
			FROM agents payee
			WHERE work_orders.onchain_order_id = $3::numeric
				AND work_orders.amount = $4::numeric
				AND work_orders.status = $5
				AND payee.id = work_orders.provider_id
				AND LOWER(payee.wallet_address) = LOWER($6)
			RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id
		`,
			string(domain.WorkOrderStatusFunded),
			event.RolledBackAt,
			event.OnchainOrderID.String(),
			event.Amount.String(),
			string(domain.WorkOrderStatusCompleted),
			event.Payee,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		err = r.recordBookkeeping(ctx, tx, bookkeepingEntry{
			WorkOrderID:    target.WorkOrderID,
			CreatedAt:      event.RolledBackAt,
			Amount:         event.Amount,
			IdempotencyKey: escrowJournalKey(target.WorkOrderID, "order_released_rollback", event.TransactionHash, event.BlockNumber, event.LogIndex, event.OnchainOrderID),
			Description:    fmt.Sprintf("Escrow order released rollback for on-chain order %s", event.OnchainOrderID.String()),
			Postings: []ledgerPosting{
				{
					AgentID:     target.PayerID,
					AccountName: accountNameEscrowLocked,
					AccountType: domain.AccountTypeAsset,
					EntryType:   domain.LedgerEntryTypeDebit,
				},
				{
					AgentID:     target.PayeeID,
					AccountName: accountNameEscrowPending,
					AccountType: domain.AccountTypeLiability,
					EntryType:   domain.LedgerEntryTypeCredit,
				},
				{
					AgentID:     target.PayeeID,
					AccountName: accountNameServiceRevenue,
					AccountType: domain.AccountTypeRevenue,
					EntryType:   domain.LedgerEntryTypeDebit,
				},
				{
					AgentID:     target.PayerID,
					AccountName: accountNameServiceExpense,
					AccountType: domain.AccountTypeExpense,
					EntryType:   domain.LedgerEntryTypeCredit,
				},
			},
		})
		if err != nil {
			return false, err
		}

		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("rollback order released: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) RecordOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		target, err := scanWorkOrderBookkeepingTarget(tx.QueryRow(ctx, `
			UPDATE work_orders
			SET
				status = $1,
				refunded_at = $2,
				updated_at = $2
			FROM agents payer
			WHERE work_orders.onchain_order_id = $3::numeric
				AND work_orders.amount = $4::numeric
				AND work_orders.status = $5
				AND payer.id = work_orders.creator_id
				AND LOWER(payer.wallet_address) = LOWER($6)
			RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id
		`,
			string(domain.WorkOrderStatusRefunded),
			event.RecordedAt,
			event.OnchainOrderID.String(),
			event.Amount.String(),
			string(domain.WorkOrderStatusFunded),
			event.Payer,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		err = r.recordBookkeeping(ctx, tx, bookkeepingEntry{
			WorkOrderID:    target.WorkOrderID,
			CreatedAt:      event.RecordedAt,
			Amount:         event.Amount,
			IdempotencyKey: escrowJournalKey(target.WorkOrderID, "order_refunded", event.TransactionHash, event.BlockNumber, event.LogIndex, event.OnchainOrderID),
			Description:    fmt.Sprintf("Escrow order refunded for on-chain order %s", event.OnchainOrderID.String()),
			Postings: []ledgerPosting{
				{
					AgentID:     target.PayeeID,
					AccountName: accountNameEscrowPending,
					AccountType: domain.AccountTypeLiability,
					EntryType:   domain.LedgerEntryTypeDebit,
				},
				{
					AgentID:     target.PayerID,
					AccountName: accountNameEscrowLocked,
					AccountType: domain.AccountTypeAsset,
					EntryType:   domain.LedgerEntryTypeCredit,
				},
			},
		})
		if err != nil {
			return false, err
		}

		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("record order refunded: %w", err)
	}

	return updated, nil
}

func (r *WorkOrderRepository) RollbackOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (bool, error) {
	updated, err := r.withTx(ctx, func(tx workOrderTx) (bool, error) {
		target, err := scanWorkOrderBookkeepingTarget(tx.QueryRow(ctx, `
			UPDATE work_orders
			SET
				status = $1,
				refunded_at = NULL,
				updated_at = $2
			FROM agents payer
			WHERE work_orders.onchain_order_id = $3::numeric
				AND work_orders.amount = $4::numeric
				AND work_orders.status = $5
				AND payer.id = work_orders.creator_id
				AND LOWER(payer.wallet_address) = LOWER($6)
			RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id
		`,
			string(domain.WorkOrderStatusFunded),
			event.RolledBackAt,
			event.OnchainOrderID.String(),
			event.Amount.String(),
			string(domain.WorkOrderStatusRefunded),
			event.Payer,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		err = r.recordBookkeeping(ctx, tx, bookkeepingEntry{
			WorkOrderID:    target.WorkOrderID,
			CreatedAt:      event.RolledBackAt,
			Amount:         event.Amount,
			IdempotencyKey: escrowJournalKey(target.WorkOrderID, "order_refunded_rollback", event.TransactionHash, event.BlockNumber, event.LogIndex, event.OnchainOrderID),
			Description:    fmt.Sprintf("Escrow order refunded rollback for on-chain order %s", event.OnchainOrderID.String()),
			Postings: []ledgerPosting{
				{
					AgentID:     target.PayerID,
					AccountName: accountNameEscrowLocked,
					AccountType: domain.AccountTypeAsset,
					EntryType:   domain.LedgerEntryTypeDebit,
				},
				{
					AgentID:     target.PayeeID,
					AccountName: accountNameEscrowPending,
					AccountType: domain.AccountTypeLiability,
					EntryType:   domain.LedgerEntryTypeCredit,
				},
			},
		})
		if err != nil {
			return false, err
		}

		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("rollback order refunded: %w", err)
	}

	return updated, nil
}

type workOrderBookkeepingTarget struct {
	WorkOrderID uuid.UUID
	PayerID     uuid.UUID
	PayeeID     uuid.UUID
}

type bookkeepingEntry struct {
	WorkOrderID    uuid.UUID
	IdempotencyKey string
	Description    string
	CreatedAt      time.Time
	Amount         big.Int
	Postings       []ledgerPosting
}

type ledgerPosting struct {
	AgentID     uuid.UUID
	AccountName string
	AccountType domain.AccountType
	EntryType   domain.LedgerEntryType
	Amount      big.Int
}

type bookkeepingJournalAnchor struct {
	SchemaVersion  string                            `json:"schema_version"`
	Kind           string                            `json:"kind"`
	JournalEntryID uuid.UUID                         `json:"journal_entry_id"`
	IdempotencyKey string                            `json:"idempotency_key"`
	WorkOrderID    uuid.UUID                         `json:"work_order_id"`
	Description    string                            `json:"description"`
	CreatedAt      string                            `json:"created_at"`
	Postings       []bookkeepingJournalAnchorPosting `json:"postings"`
}

type bookkeepingJournalAnchorPosting struct {
	AccountName string                 `json:"account_name"`
	AccountType domain.AccountType     `json:"account_type"`
	EntryType   domain.LedgerEntryType `json:"entry_type"`
	Amount      string                 `json:"amount"`
	AgentID     uuid.UUID              `json:"agent_id"`
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

func (r *WorkOrderRepository) recordBookkeeping(ctx context.Context, tx workOrderTx, entry bookkeepingEntry) error {
	journalID, err := r.nextID()
	if err != nil {
		return fmt.Errorf("generate journal entry id: %w", err)
	}

	storageCID, err := r.uploadBookkeepingJournal(ctx, journalID, entry)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
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
		journalID,
		entry.IdempotencyKey,
		entry.WorkOrderID,
		entry.Description,
		storageCID,
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

		if err := r.recordLedgerPosting(ctx, tx, accountID, journalID, posting, entry.Amount, entry.CreatedAt); err != nil {
			return err
		}
	}

	return nil
}

func (r *WorkOrderRepository) uploadBookkeepingJournal(ctx context.Context, journalID uuid.UUID, entry bookkeepingEntry) (string, error) {
	if r.storageUploader == nil {
		return "", fmt.Errorf("%w: escrow journal storage uploader is not configured", domain.ErrStorage)
	}

	payload := bookkeepingJournalAnchor{
		SchemaVersion:  "1.0",
		Kind:           "escrow_journal_entry",
		JournalEntryID: journalID,
		IdempotencyKey: entry.IdempotencyKey,
		WorkOrderID:    entry.WorkOrderID,
		Description:    entry.Description,
		CreatedAt:      entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		Postings:       make([]bookkeepingJournalAnchorPosting, 0, len(entry.Postings)),
	}
	amount := entry.Amount.String()
	for _, posting := range entry.Postings {
		payload.Postings = append(payload.Postings, bookkeepingJournalAnchorPosting{
			AccountName: posting.AccountName,
			AccountType: posting.AccountType,
			EntryType:   posting.EntryType,
			Amount:      amount,
			AgentID:     posting.AgentID,
		})
	}

	uploadOutput, err := r.storageUploader.UploadJSON(ctx, payload)
	if err != nil {
		return "", fmt.Errorf("%w: upload escrow journal entry: %w", domain.ErrStorage, err)
	}
	if uploadOutput == nil || strings.TrimSpace(uploadOutput.RootHash) == "" {
		return "", fmt.Errorf("%w: upload escrow journal entry returned empty root hash", domain.ErrStorage)
	}

	return uploadOutput.RootHash, nil
}

func (r *WorkOrderRepository) findOrCreateAccount(ctx context.Context, tx workOrderTx, posting ledgerPosting, createdAt time.Time) (uuid.UUID, error) {
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

func (r *WorkOrderRepository) nextID() (uuid.UUID, error) {
	if r.newID != nil {
		return r.newID()
	}

	return uuid.NewV7()
}

func escrowJournalKey(workOrderID uuid.UUID, action string, transactionHash string, blockNumber string, logIndex string, onchainOrderID big.Int) string {
	eventID := strings.TrimSpace(transactionHash)
	if eventID != "" {
		blockNumber = strings.TrimSpace(blockNumber)
		logIndex = strings.TrimSpace(logIndex)
		if blockNumber != "" || logIndex != "" {
			eventID = fmt.Sprintf("%s:%s:%s", eventID, blockNumber, logIndex)
		}
	} else {
		eventID = onchainOrderID.String()
	}

	return fmt.Sprintf("work_order:%s:%s:%s", workOrderID, action, eventID)
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

func scanWorkOrderBookkeepingTarget(row pgx.Row) (*workOrderBookkeepingTarget, error) {
	var target workOrderBookkeepingTarget
	if err := row.Scan(&target.WorkOrderID, &target.PayerID, &target.PayeeID); err != nil {
		return nil, err
	}

	return &target, nil
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
