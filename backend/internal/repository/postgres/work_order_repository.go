package postgres

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workOrderDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type WorkOrderRepository struct {
	db workOrderDB
}

func NewWorkOrderRepository(db *pgxpool.Pool) *WorkOrderRepository {
	return &WorkOrderRepository{db: db}
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
			completed_at,
			refunded_at,
			onchain_order_id,
			order_tx_hash,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5::numeric, $6, $7, $8, $9, $10, $11, $12, $13::numeric, $14, $15, $16
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

func (r *WorkOrderRepository) RecordOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (bool, error) {
	commandTag, err := r.db.Exec(ctx, `
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
	)
	if err != nil {
		return false, fmt.Errorf("record order created: %w", err)
	}

	return commandTag.RowsAffected() > 0, nil
}

func (r *WorkOrderRepository) RollbackOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (bool, error) {
	commandTag, err := r.db.Exec(ctx, `
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
	)
	if err != nil {
		return false, fmt.Errorf("rollback order created: %w", err)
	}

	return commandTag.RowsAffected() > 0, nil
}

func (r *WorkOrderRepository) RecordOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (bool, error) {
	commandTag, err := r.db.Exec(ctx, `
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
	`,
		string(domain.WorkOrderStatusCompleted),
		event.RecordedAt,
		event.OnchainOrderID.String(),
		event.Amount.String(),
		string(domain.WorkOrderStatusFunded),
		event.Payee,
	)
	if err != nil {
		return false, fmt.Errorf("record order released: %w", err)
	}

	return commandTag.RowsAffected() > 0, nil
}

func (r *WorkOrderRepository) RollbackOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (bool, error) {
	commandTag, err := r.db.Exec(ctx, `
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
	`,
		string(domain.WorkOrderStatusFunded),
		event.RolledBackAt,
		event.OnchainOrderID.String(),
		event.Amount.String(),
		string(domain.WorkOrderStatusCompleted),
		event.Payee,
	)
	if err != nil {
		return false, fmt.Errorf("rollback order released: %w", err)
	}

	return commandTag.RowsAffected() > 0, nil
}

func (r *WorkOrderRepository) RecordOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (bool, error) {
	commandTag, err := r.db.Exec(ctx, `
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
	`,
		string(domain.WorkOrderStatusRefunded),
		event.RecordedAt,
		event.OnchainOrderID.String(),
		event.Amount.String(),
		string(domain.WorkOrderStatusFunded),
		event.Payer,
	)
	if err != nil {
		return false, fmt.Errorf("record order refunded: %w", err)
	}

	return commandTag.RowsAffected() > 0, nil
}

func (r *WorkOrderRepository) RollbackOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (bool, error) {
	commandTag, err := r.db.Exec(ctx, `
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
	`,
		string(domain.WorkOrderStatusFunded),
		event.RolledBackAt,
		event.OnchainOrderID.String(),
		event.Amount.String(),
		string(domain.WorkOrderStatusRefunded),
		event.Payer,
	)
	if err != nil {
		return false, fmt.Errorf("rollback order refunded: %w", err)
	}

	return commandTag.RowsAffected() > 0, nil
}

func scanWorkOrder(row pgx.Row) (*domain.WorkOrder, error) {
	var workOrder domain.WorkOrder
	var amountText string
	var status string
	var deliverableCID pgtype.Text
	var completedAt pgtype.Timestamptz
	var refundedAt pgtype.Timestamptz
	var onchainOrderIDText pgtype.Text
	var orderTxHash pgtype.Text

	if err := row.Scan(
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
		&completedAt,
		&refundedAt,
		&onchainOrderIDText,
		&orderTxHash,
		&workOrder.CreatedAt,
		&workOrder.UpdatedAt,
	); err != nil {
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
