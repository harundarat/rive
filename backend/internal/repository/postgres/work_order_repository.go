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

// workOrderColumns is the single source of truth for the work_orders projection.
// scanWorkOrderWithExtra reads these in this exact order; workOrderColumnList
// renders them (optionally table-qualified for joined queries) so the three
// finders never hand-maintain the list separately.
var workOrderColumns = []string{
	"id",
	"idempotency_key",
	"creator_id",
	"provider_id",
	"amount::text",
	"status",
	"spec_hash",
	"spec_version",
	"spec_tx_hash",
	"deliverable_cid",
	"delivered_at",
	"completed_at",
	"refunded_at",
	"onchain_order_id::text",
	"order_tx_hash",
	"release_tx_hash",
	"refund_tx_hash",
	"created_at",
	"updated_at",
}

// workOrderColumnList renders the work_orders projection, prefixing each column
// with prefix (e.g. "work_orders." to disambiguate a join).
func workOrderColumnList(prefix string) string {
	columns := make([]string, len(workOrderColumns))
	for i, column := range workOrderColumns {
		columns[i] = prefix + column
	}

	return strings.Join(columns, ", ")
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
		SELECT `+workOrderColumnList("")+`
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
		SELECT `+workOrderColumnList("")+`
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
		SELECT `+workOrderColumnList("work_orders.")+`,
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
			release_tx_hash,
			refund_tx_hash,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5::numeric, $6, $7, $8, $9, $10, $11, $12, $13, $14::numeric, $15, $16, $17, $18, $19
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
		stringValue(workOrder.ReleaseTxHash),
		stringValue(workOrder.RefundTxHash),
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
	var releaseTxHash pgtype.Text
	var refundTxHash pgtype.Text

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
		&releaseTxHash,
		&refundTxHash,
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
	if releaseTxHash.Valid {
		value := releaseTxHash.String
		workOrder.ReleaseTxHash = &value
	}
	if refundTxHash.Valid {
		value := refundTxHash.String
		workOrder.RefundTxHash = &value
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
