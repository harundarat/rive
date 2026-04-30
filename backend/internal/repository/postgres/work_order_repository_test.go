package postgres

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeWorkOrderDB struct {
	sql          string
	args         []any
	rowsAffected int64
	err          error
}

func (db *fakeWorkOrderDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.sql = sql
	db.args = args
	return fakeWorkOrderRow{}
}

func (db *fakeWorkOrderDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.sql = sql
	db.args = args
	if db.err != nil {
		return pgconn.CommandTag{}, db.err
	}

	return pgconn.NewCommandTag("UPDATE " + big.NewInt(db.rowsAffected).String()), nil
}

type fakeWorkOrderRow struct{}

func (fakeWorkOrderRow) Scan(dest ...any) error {
	return errors.New("unexpected query row scan")
}

func TestWorkOrderRepositoryRecordOrderCreatedUsesFundedStatusAndPartyPredicates(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 1}
	repo := &WorkOrderRepository{db: db}
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	recordedAt := time.Date(2026, 4, 22, 10, 30, 0, 0, time.UTC)

	updated, err := repo.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		SpecHash:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Payer:           "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		Payee:           "0x1111111111111111111111111111111111111111",
		Amount:          bigIntFromStringForRepositoryTest("1000000"),
		OnchainOrderID:  largeOrderID,
		TransactionHash: "0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70",
		RecordedAt:      recordedAt,
	})
	if err != nil {
		t.Fatalf("RecordOrderCreated returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	for _, expected := range []string{
		"status = $1",
		"onchain_order_id = $3::numeric",
		"order_tx_hash = $4",
		"LOWER(payer.wallet_address) = LOWER($8)",
		"LOWER(payee.wallet_address) = LOWER($9)",
	} {
		if !strings.Contains(db.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, db.sql)
		}
	}
	for _, unexpected := range []string{"funded_at", "::bigint", "funding_tx_hash"} {
		if strings.Contains(db.sql, unexpected) {
			t.Fatalf("expected SQL not to contain %q, got %s", unexpected, db.sql)
		}
	}

	assertArg(t, db.args, 0, string(domain.WorkOrderStatusFunded))
	assertArg(t, db.args, 1, recordedAt)
	assertArg(t, db.args, 2, largeOrderID.String())
	assertArg(t, db.args, 6, string(domain.WorkOrderStatusDraft))
	assertArg(t, db.args, 7, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73")
	assertArg(t, db.args, 8, "0x1111111111111111111111111111111111111111")
}

func TestWorkOrderRepositoryRollbackOrderCreatedClearsAtomicFundingFields(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 1}
	repo := &WorkOrderRepository{db: db}
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	rolledBackAt := time.Date(2026, 4, 22, 10, 45, 0, 0, time.UTC)

	updated, err := repo.RollbackOrderCreated(context.Background(), domain.OrderCreatedWorkOrderRollback{
		SpecHash:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Payer:           "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		Payee:           "0x1111111111111111111111111111111111111111",
		Amount:          bigIntFromStringForRepositoryTest("1000000"),
		OnchainOrderID:  largeOrderID,
		TransactionHash: "0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70",
		RolledBackAt:    rolledBackAt,
	})
	if err != nil {
		t.Fatalf("RollbackOrderCreated returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	for _, expected := range []string{
		"status = $1",
		"onchain_order_id = NULL",
		"order_tx_hash = NULL",
		"work_orders.onchain_order_id = $5::numeric",
		"work_orders.order_tx_hash = $6",
		"LOWER(payer.wallet_address) = LOWER($8)",
		"LOWER(payee.wallet_address) = LOWER($9)",
	} {
		if !strings.Contains(db.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, db.sql)
		}
	}
	for _, unexpected := range []string{"funded_at", "::bigint", "funding_tx_hash"} {
		if strings.Contains(db.sql, unexpected) {
			t.Fatalf("expected SQL not to contain %q, got %s", unexpected, db.sql)
		}
	}

	assertArg(t, db.args, 0, string(domain.WorkOrderStatusDraft))
	assertArg(t, db.args, 1, rolledBackAt)
	assertArg(t, db.args, 4, largeOrderID.String())
	assertArg(t, db.args, 6, string(domain.WorkOrderStatusFunded))
	assertArg(t, db.args, 7, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73")
	assertArg(t, db.args, 8, "0x1111111111111111111111111111111111111111")
}

func TestWorkOrderRepositoryRecordOrderReleasedSetsCompletedStatusAndPayeePredicate(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 1}
	repo := &WorkOrderRepository{db: db}
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	recordedAt := time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC)

	updated, err := repo.RecordOrderReleased(context.Background(), domain.OrderReleasedWorkOrderUpdate{
		Payee:          "0x933A54D5D7A6C0c9E6318395A74CB99aC1C56934",
		Amount:         bigIntFromStringForRepositoryTest("1000000"),
		OnchainOrderID: largeOrderID,
		RecordedAt:     recordedAt,
	})
	if err != nil {
		t.Fatalf("RecordOrderReleased returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	for _, expected := range []string{
		"status = $1",
		"completed_at = $2",
		"updated_at = $2",
		"work_orders.onchain_order_id = $3::numeric",
		"work_orders.amount = $4::numeric",
		"payee.id = work_orders.provider_id",
		"LOWER(payee.wallet_address) = LOWER($6)",
	} {
		if !strings.Contains(db.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, db.sql)
		}
	}

	assertArg(t, db.args, 0, string(domain.WorkOrderStatusCompleted))
	assertArg(t, db.args, 1, recordedAt)
	assertArg(t, db.args, 2, largeOrderID.String())
	assertArg(t, db.args, 3, "1000000")
	assertArg(t, db.args, 4, string(domain.WorkOrderStatusFunded))
	assertArg(t, db.args, 5, "0x933A54D5D7A6C0c9E6318395A74CB99aC1C56934")
}

func TestWorkOrderRepositoryRollbackOrderReleasedRestoresFundedStatus(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 1}
	repo := &WorkOrderRepository{db: db}
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	rolledBackAt := time.Date(2026, 4, 22, 11, 5, 0, 0, time.UTC)

	updated, err := repo.RollbackOrderReleased(context.Background(), domain.OrderReleasedWorkOrderRollback{
		Payee:          "0x933A54D5D7A6C0c9E6318395A74CB99aC1C56934",
		Amount:         bigIntFromStringForRepositoryTest("1000000"),
		OnchainOrderID: largeOrderID,
		RolledBackAt:   rolledBackAt,
	})
	if err != nil {
		t.Fatalf("RollbackOrderReleased returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	for _, expected := range []string{
		"status = $1",
		"completed_at = NULL",
		"updated_at = $2",
		"work_orders.status = $5",
		"payee.id = work_orders.provider_id",
	} {
		if !strings.Contains(db.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, db.sql)
		}
	}

	assertArg(t, db.args, 0, string(domain.WorkOrderStatusFunded))
	assertArg(t, db.args, 1, rolledBackAt)
	assertArg(t, db.args, 2, largeOrderID.String())
	assertArg(t, db.args, 3, "1000000")
	assertArg(t, db.args, 4, string(domain.WorkOrderStatusCompleted))
	assertArg(t, db.args, 5, "0x933A54D5D7A6C0c9E6318395A74CB99aC1C56934")
}

func TestWorkOrderRepositoryRecordOrderRefundedSetsRefundedStatusAndPayerPredicate(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 1}
	repo := &WorkOrderRepository{db: db}
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	recordedAt := time.Date(2026, 4, 22, 11, 10, 0, 0, time.UTC)

	updated, err := repo.RecordOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderUpdate{
		Payer:          "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		Amount:         bigIntFromStringForRepositoryTest("1000000"),
		OnchainOrderID: largeOrderID,
		RecordedAt:     recordedAt,
	})
	if err != nil {
		t.Fatalf("RecordOrderRefunded returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	for _, expected := range []string{
		"status = $1",
		"refunded_at = $2",
		"updated_at = $2",
		"work_orders.onchain_order_id = $3::numeric",
		"work_orders.amount = $4::numeric",
		"payer.id = work_orders.creator_id",
		"LOWER(payer.wallet_address) = LOWER($6)",
	} {
		if !strings.Contains(db.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, db.sql)
		}
	}

	assertArg(t, db.args, 0, string(domain.WorkOrderStatusRefunded))
	assertArg(t, db.args, 1, recordedAt)
	assertArg(t, db.args, 2, largeOrderID.String())
	assertArg(t, db.args, 3, "1000000")
	assertArg(t, db.args, 4, string(domain.WorkOrderStatusFunded))
	assertArg(t, db.args, 5, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73")
}

func TestWorkOrderRepositoryRollbackOrderRefundedRestoresFundedStatus(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 1}
	repo := &WorkOrderRepository{db: db}
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	rolledBackAt := time.Date(2026, 4, 22, 11, 15, 0, 0, time.UTC)

	updated, err := repo.RollbackOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderRollback{
		Payer:          "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		Amount:         bigIntFromStringForRepositoryTest("1000000"),
		OnchainOrderID: largeOrderID,
		RolledBackAt:   rolledBackAt,
	})
	if err != nil {
		t.Fatalf("RollbackOrderRefunded returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	for _, expected := range []string{
		"status = $1",
		"refunded_at = NULL",
		"updated_at = $2",
		"work_orders.status = $5",
		"payer.id = work_orders.creator_id",
	} {
		if !strings.Contains(db.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, db.sql)
		}
	}

	assertArg(t, db.args, 0, string(domain.WorkOrderStatusFunded))
	assertArg(t, db.args, 1, rolledBackAt)
	assertArg(t, db.args, 2, largeOrderID.String())
	assertArg(t, db.args, 3, "1000000")
	assertArg(t, db.args, 4, string(domain.WorkOrderStatusRefunded))
	assertArg(t, db.args, 5, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73")
}

func TestWorkOrderRepositoryFindDeliveryTargetUsesOnchainOrderIDAndPayeeJoin(t *testing.T) {
	db := &fakeWorkOrderDB{}
	repo := &WorkOrderRepository{db: db}
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")

	_, err := repo.FindDeliveryTargetByOnchainOrderID(context.Background(), largeOrderID)
	if err == nil {
		t.Fatal("expected fake scan error")
	}

	for _, expected := range []string{
		"payee.wallet_address",
		"INNER JOIN agents payee ON payee.id = work_orders.provider_id",
		"WHERE work_orders.onchain_order_id = $1::numeric",
	} {
		if !strings.Contains(db.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, db.sql)
		}
	}
	assertArg(t, db.args, 0, largeOrderID.String())
}

func TestWorkOrderRepositorySubmitDeliveryStoresDeliverableCIDWithFundedAndUndeliveredPredicates(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 1}
	repo := &WorkOrderRepository{db: db}
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	deliveredAt := time.Date(2026, 4, 22, 11, 20, 0, 0, time.UTC)
	deliveryHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	updated, err := repo.SubmitDelivery(context.Background(), domain.WorkOrderDeliveryUpdate{
		OnchainOrderID: largeOrderID,
		DeliveryHash:   deliveryHash,
		DeliveredAt:    deliveredAt,
	})
	if err != nil {
		t.Fatalf("SubmitDelivery returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	for _, expected := range []string{
		"deliverable_cid = $1",
		"delivered_at = $2",
		"updated_at = $2",
		"onchain_order_id = $3::numeric",
		"status = $4",
		"deliverable_cid IS NULL",
	} {
		if !strings.Contains(db.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, db.sql)
		}
	}

	assertArg(t, db.args, 0, deliveryHash)
	assertArg(t, db.args, 1, deliveredAt)
	assertArg(t, db.args, 2, largeOrderID.String())
	assertArg(t, db.args, 3, string(domain.WorkOrderStatusFunded))
}

func TestWorkOrderRepositoryRecordOrderCreatedReturnsFalseWhenPredicateDoesNotMatch(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 0}
	repo := &WorkOrderRepository{db: db}

	updated, err := repo.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		SpecHash:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Payer:           "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		Payee:           "0x1111111111111111111111111111111111111111",
		Amount:          bigIntFromStringForRepositoryTest("1000000"),
		OnchainOrderID:  bigIntFromStringForRepositoryTest("1"),
		TransactionHash: "0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70",
		RecordedAt:      time.Date(2026, 4, 22, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordOrderCreated returned error: %v", err)
	}
	if updated {
		t.Fatal("expected updated=false")
	}
}

func assertArg[T comparable](t *testing.T, args []any, index int, expected T) {
	t.Helper()

	if len(args) <= index {
		t.Fatalf("expected arg %d to exist, got %d args", index, len(args))
	}
	if args[index] != expected {
		t.Fatalf("expected arg %d to be %v, got %v", index, expected, args[index])
	}
}

func bigIntFromStringForRepositoryTest(value string) big.Int {
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic("invalid test big.Int")
	}

	return *amount
}
