package postgres

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	repositoryTestWorkOrderID = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a1f")
	repositoryTestPayerID     = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a20")
	repositoryTestPayeeID     = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a21")
)

const (
	repositoryTestStorageRootHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	repositoryTestStorageTxHash   = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeSQLOperation struct {
	sql  string
	args []any
}

type fakeWorkOrderStorage struct {
	data  any
	err   error
	calls int
}

func (s *fakeWorkOrderStorage) UploadJSON(ctx context.Context, data any) (*domain.ZGUploadOutput, error) {
	s.calls++
	s.data = data
	if s.err != nil {
		return nil, s.err
	}

	return &domain.ZGUploadOutput{RootHash: repositoryTestStorageRootHash, TxHash: repositoryTestStorageTxHash}, nil
}

type fakeWorkOrderDB struct {
	sql           string
	args          []any
	rowsAffected  int64
	err           error
	target        *workOrderBookkeepingTarget
	operations    []fakeSQLOperation
	beginCalls    int
	commitCalls   int
	rollbackCalls int
	accountIDs    map[string]uuid.UUID
}

func (db *fakeWorkOrderDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.record(sql, args...)
	return fakeWorkOrderRow{err: errors.New("unexpected query row scan")}
}

func (db *fakeWorkOrderDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.record(sql, args...)
	if db.err != nil {
		return pgconn.CommandTag{}, db.err
	}

	return pgconn.NewCommandTag("UPDATE " + big.NewInt(db.rowsAffected).String()), nil
}

func (db *fakeWorkOrderDB) beginTx(ctx context.Context) (workOrderTx, error) {
	db.beginCalls++

	return &fakeWorkOrderTx{db: db}, nil
}

func (db *fakeWorkOrderDB) record(sql string, args ...any) {
	db.sql = sql
	db.args = args
	db.operations = append(db.operations, fakeSQLOperation{sql: sql, args: args})
}

func (db *fakeWorkOrderDB) operation(t *testing.T, index int) fakeSQLOperation {
	t.Helper()

	if len(db.operations) <= index {
		t.Fatalf("expected operation %d to exist, got %d operations", index, len(db.operations))
	}

	return db.operations[index]
}

func (db *fakeWorkOrderDB) containsSQL(fragment string) bool {
	for _, operation := range db.operations {
		if strings.Contains(operation.sql, fragment) {
			return true
		}
	}

	return false
}

func (db *fakeWorkOrderDB) operationsContaining(fragment string) []fakeSQLOperation {
	var matches []fakeSQLOperation
	for _, operation := range db.operations {
		if strings.Contains(operation.sql, fragment) {
			matches = append(matches, operation)
		}
	}

	return matches
}

type fakeWorkOrderTx struct {
	db *fakeWorkOrderDB
}

func (tx *fakeWorkOrderTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	tx.db.record(sql, args...)
	if tx.db.err != nil {
		return fakeWorkOrderRow{err: tx.db.err}
	}

	switch {
	case strings.Contains(sql, "UPDATE work_orders"):
		if tx.db.target == nil {
			return fakeWorkOrderRow{err: pgx.ErrNoRows}
		}

		return fakeWorkOrderRow{values: []any{
			tx.db.target.WorkOrderID,
			tx.db.target.PayerID,
			tx.db.target.PayeeID,
		}}
	case strings.Contains(sql, "INSERT INTO accounts"):
		if tx.db.accountIDs == nil {
			tx.db.accountIDs = map[string]uuid.UUID{}
		}

		accountID := args[0].(uuid.UUID)
		key := args[1].(uuid.UUID).String() + ":" + args[2].(string)
		if existingID, ok := tx.db.accountIDs[key]; ok {
			accountID = existingID
		} else {
			tx.db.accountIDs[key] = accountID
		}

		return fakeWorkOrderRow{values: []any{accountID}}
	default:
		return fakeWorkOrderRow{err: errors.New("unexpected transaction query row scan")}
	}
}

func (tx *fakeWorkOrderTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.db.record(sql, args...)
	if tx.db.err != nil {
		return pgconn.CommandTag{}, tx.db.err
	}

	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (tx *fakeWorkOrderTx) Commit(ctx context.Context) error {
	tx.db.commitCalls++

	return nil
}

func (tx *fakeWorkOrderTx) Rollback(ctx context.Context) error {
	tx.db.rollbackCalls++

	return nil
}

type fakeWorkOrderRow struct {
	values []any
	err    error
}

func (row fakeWorkOrderRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return errors.New("unexpected scan destination count")
	}

	for i := range dest {
		switch value := dest[i].(type) {
		case *uuid.UUID:
			*value = row.values[i].(uuid.UUID)
		default:
			return errors.New("unsupported scan destination")
		}
	}

	return nil
}

func newEventWorkOrderRepository(db *fakeWorkOrderDB) *WorkOrderRepository {
	return newEventWorkOrderRepositoryWithStorage(db, &fakeWorkOrderStorage{})
}

func newEventWorkOrderRepositoryWithStorage(db *fakeWorkOrderDB, storage *fakeWorkOrderStorage) *WorkOrderRepository {
	return &WorkOrderRepository{
		db:              db,
		storageUploader: storage,
		beginTx:         db.beginTx,
		newID:           uuid.NewV7,
	}
}

func validBookkeepingTarget() *workOrderBookkeepingTarget {
	return &workOrderBookkeepingTarget{
		WorkOrderID: repositoryTestWorkOrderID,
		PayerID:     repositoryTestPayerID,
		PayeeID:     repositoryTestPayeeID,
	}
}

func TestWorkOrderRepositoryRecordOrderCreatedUsesFundedStatusAndPartyPredicates(t *testing.T) {
	db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
	repo := newEventWorkOrderRepository(db)
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

	updateOp := db.operation(t, 0)
	for _, expected := range []string{
		"status = $1",
		"onchain_order_id = $3::numeric",
		"order_tx_hash = $4",
		"LOWER(payer.wallet_address) = LOWER($8)",
		"LOWER(payee.wallet_address) = LOWER($9)",
		"RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id",
	} {
		if !strings.Contains(updateOp.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, updateOp.sql)
		}
	}
	for _, unexpected := range []string{"funded_at", "::bigint", "funding_tx_hash"} {
		if strings.Contains(updateOp.sql, unexpected) {
			t.Fatalf("expected SQL not to contain %q, got %s", unexpected, updateOp.sql)
		}
	}

	assertArg(t, updateOp.args, 0, string(domain.WorkOrderStatusFunded))
	assertArg(t, updateOp.args, 1, recordedAt)
	assertArg(t, updateOp.args, 2, largeOrderID.String())
	assertArg(t, updateOp.args, 6, string(domain.WorkOrderStatusDraft))
	assertArg(t, updateOp.args, 7, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73")
	assertArg(t, updateOp.args, 8, "0x1111111111111111111111111111111111111111")
}

func TestWorkOrderRepositoryRollbackOrderCreatedClearsAtomicFundingFields(t *testing.T) {
	db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
	repo := newEventWorkOrderRepository(db)
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

	updateOp := db.operation(t, 0)
	for _, expected := range []string{
		"status = $1",
		"onchain_order_id = NULL",
		"order_tx_hash = NULL",
		"work_orders.onchain_order_id = $5::numeric",
		"work_orders.order_tx_hash = $6",
		"LOWER(payer.wallet_address) = LOWER($8)",
		"LOWER(payee.wallet_address) = LOWER($9)",
		"RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id",
	} {
		if !strings.Contains(updateOp.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, updateOp.sql)
		}
	}
	for _, unexpected := range []string{"funded_at", "::bigint", "funding_tx_hash"} {
		if strings.Contains(updateOp.sql, unexpected) {
			t.Fatalf("expected SQL not to contain %q, got %s", unexpected, updateOp.sql)
		}
	}

	assertArg(t, updateOp.args, 0, string(domain.WorkOrderStatusDraft))
	assertArg(t, updateOp.args, 1, rolledBackAt)
	assertArg(t, updateOp.args, 4, largeOrderID.String())
	assertArg(t, updateOp.args, 6, string(domain.WorkOrderStatusFunded))
	assertArg(t, updateOp.args, 7, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73")
	assertArg(t, updateOp.args, 8, "0x1111111111111111111111111111111111111111")
}

func TestWorkOrderRepositoryRecordOrderReleasedSetsCompletedStatusAndPayeePredicate(t *testing.T) {
	db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
	repo := newEventWorkOrderRepository(db)
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

	updateOp := db.operation(t, 0)
	for _, expected := range []string{
		"status = $1",
		"completed_at = $2",
		"updated_at = $2",
		"work_orders.onchain_order_id = $3::numeric",
		"work_orders.amount = $4::numeric",
		"payee.id = work_orders.provider_id",
		"LOWER(payee.wallet_address) = LOWER($6)",
		"RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id",
	} {
		if !strings.Contains(updateOp.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, updateOp.sql)
		}
	}

	assertArg(t, updateOp.args, 0, string(domain.WorkOrderStatusCompleted))
	assertArg(t, updateOp.args, 1, recordedAt)
	assertArg(t, updateOp.args, 2, largeOrderID.String())
	assertArg(t, updateOp.args, 3, "1000000")
	assertArg(t, updateOp.args, 4, string(domain.WorkOrderStatusFunded))
	assertArg(t, updateOp.args, 5, "0x933A54D5D7A6C0c9E6318395A74CB99aC1C56934")
}

func TestWorkOrderRepositoryRollbackOrderReleasedRestoresFundedStatus(t *testing.T) {
	db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
	repo := newEventWorkOrderRepository(db)
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

	updateOp := db.operation(t, 0)
	for _, expected := range []string{
		"status = $1",
		"completed_at = NULL",
		"updated_at = $2",
		"work_orders.status = $5",
		"payee.id = work_orders.provider_id",
		"RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id",
	} {
		if !strings.Contains(updateOp.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, updateOp.sql)
		}
	}

	assertArg(t, updateOp.args, 0, string(domain.WorkOrderStatusFunded))
	assertArg(t, updateOp.args, 1, rolledBackAt)
	assertArg(t, updateOp.args, 2, largeOrderID.String())
	assertArg(t, updateOp.args, 3, "1000000")
	assertArg(t, updateOp.args, 4, string(domain.WorkOrderStatusCompleted))
	assertArg(t, updateOp.args, 5, "0x933A54D5D7A6C0c9E6318395A74CB99aC1C56934")
}

func TestWorkOrderRepositoryRecordOrderRefundedSetsRefundedStatusAndPayerPredicate(t *testing.T) {
	db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
	repo := newEventWorkOrderRepository(db)
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

	updateOp := db.operation(t, 0)
	for _, expected := range []string{
		"status = $1",
		"refunded_at = $2",
		"updated_at = $2",
		"work_orders.onchain_order_id = $3::numeric",
		"work_orders.amount = $4::numeric",
		"payer.id = work_orders.creator_id",
		"LOWER(payer.wallet_address) = LOWER($6)",
		"RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id",
	} {
		if !strings.Contains(updateOp.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, updateOp.sql)
		}
	}

	assertArg(t, updateOp.args, 0, string(domain.WorkOrderStatusRefunded))
	assertArg(t, updateOp.args, 1, recordedAt)
	assertArg(t, updateOp.args, 2, largeOrderID.String())
	assertArg(t, updateOp.args, 3, "1000000")
	assertArg(t, updateOp.args, 4, string(domain.WorkOrderStatusFunded))
	assertArg(t, updateOp.args, 5, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73")
}

func TestWorkOrderRepositoryRollbackOrderRefundedRestoresFundedStatus(t *testing.T) {
	db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
	repo := newEventWorkOrderRepository(db)
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

	updateOp := db.operation(t, 0)
	for _, expected := range []string{
		"status = $1",
		"refunded_at = NULL",
		"updated_at = $2",
		"work_orders.status = $5",
		"payer.id = work_orders.creator_id",
		"RETURNING work_orders.id, work_orders.creator_id, work_orders.provider_id",
	} {
		if !strings.Contains(updateOp.sql, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, updateOp.sql)
		}
	}

	assertArg(t, updateOp.args, 0, string(domain.WorkOrderStatusFunded))
	assertArg(t, updateOp.args, 1, rolledBackAt)
	assertArg(t, updateOp.args, 2, largeOrderID.String())
	assertArg(t, updateOp.args, 3, "1000000")
	assertArg(t, updateOp.args, 4, string(domain.WorkOrderStatusRefunded))
	assertArg(t, updateOp.args, 5, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73")
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
	repo := newEventWorkOrderRepository(db)

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
	if db.containsSQL("journal_entries") || db.containsSQL("ledger_entries") || db.containsSQL("UPDATE accounts") {
		t.Fatalf("expected no bookkeeping writes when work order predicate does not match, got %+v", db.operations)
	}
	if db.beginCalls != 1 || db.commitCalls != 1 || db.rollbackCalls != 0 {
		t.Fatalf("expected clean transaction commit for no-op, got begin=%d commit=%d rollback=%d", db.beginCalls, db.commitCalls, db.rollbackCalls)
	}
}

func TestWorkOrderRepositoryEscrowBookkeepingRollsBackWhenStorageUploadFails(t *testing.T) {
	db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
	storageErr := errors.New("0g unavailable")
	storage := &fakeWorkOrderStorage{err: storageErr}
	repo := newEventWorkOrderRepositoryWithStorage(db, storage)

	updated, err := repo.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		SpecHash:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Payer:           "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		Payee:           "0x1111111111111111111111111111111111111111",
		Amount:          bigIntFromStringForRepositoryTest("1000000"),
		OnchainOrderID:  bigIntFromStringForRepositoryTest("1"),
		TransactionHash: "0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70",
		BlockNumber:     "0x123",
		LogIndex:        "0x4",
		RecordedAt:      time.Date(2026, 4, 22, 10, 30, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected storage error")
	}
	if updated {
		t.Fatal("expected updated=false")
	}
	if !errors.Is(err, domain.ErrStorage) || !errors.Is(err, storageErr) {
		t.Fatalf("expected storage error wrapping original error, got %v", err)
	}
	if storage.calls != 1 {
		t.Fatalf("expected one storage upload, got %d", storage.calls)
	}
	if db.containsSQL("INSERT INTO journal_entries") ||
		db.containsSQL("INSERT INTO ledger_entries") ||
		db.containsSQL("INSERT INTO accounts") ||
		db.containsSQL("UPDATE accounts") {
		t.Fatalf("expected no bookkeeping writes after storage failure, got %+v", db.operations)
	}
	if db.beginCalls != 1 || db.commitCalls != 0 || db.rollbackCalls != 1 {
		t.Fatalf("expected rolled back transaction, got begin=%d commit=%d rollback=%d", db.beginCalls, db.commitCalls, db.rollbackCalls)
	}
}

func TestWorkOrderRepositoryEscrowBookkeepingPostings(t *testing.T) {
	amount := bigIntFromStringForRepositoryTest("1000000")
	orderID := bigIntFromStringForRepositoryTest("123")
	txHash := "0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70"
	blockNumber := "0x123"
	logIndex := "0x4"
	eventID := txHash + ":" + blockNumber + ":" + logIndex
	eventTime := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name            string
		run             func(*WorkOrderRepository) (bool, error)
		journalKey      string
		ledgerTypes     []string
		balanceDeltas   []string
		accountNames    []string
		accountTypes    []string
		journalContains string
	}{
		{
			name: "order created",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
					SpecHash:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Payer:           "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
					Payee:           "0x1111111111111111111111111111111111111111",
					Amount:          amount,
					OnchainOrderID:  orderID,
					TransactionHash: txHash,
					BlockNumber:     blockNumber,
					LogIndex:        logIndex,
					RecordedAt:      eventTime,
				})
			},
			journalKey:      "work_order:" + repositoryTestWorkOrderID.String() + ":order_created:" + eventID,
			ledgerTypes:     []string{string(domain.LedgerEntryTypeDebit), string(domain.LedgerEntryTypeCredit)},
			balanceDeltas:   []string{"1000000", "1000000"},
			accountNames:    []string{accountNameEscrowLocked, accountNameEscrowPending},
			accountTypes:    []string{string(domain.AccountTypeAsset), string(domain.AccountTypeLiability)},
			journalContains: "Escrow order created",
		},
		{
			name: "order created rollback",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.RollbackOrderCreated(context.Background(), domain.OrderCreatedWorkOrderRollback{
					SpecHash:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Payer:           "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
					Payee:           "0x1111111111111111111111111111111111111111",
					Amount:          amount,
					OnchainOrderID:  orderID,
					TransactionHash: txHash,
					BlockNumber:     blockNumber,
					LogIndex:        logIndex,
					RolledBackAt:    eventTime,
				})
			},
			journalKey:      "work_order:" + repositoryTestWorkOrderID.String() + ":order_created_rollback:" + eventID,
			ledgerTypes:     []string{string(domain.LedgerEntryTypeDebit), string(domain.LedgerEntryTypeCredit)},
			balanceDeltas:   []string{"-1000000", "-1000000"},
			accountNames:    []string{accountNameEscrowPending, accountNameEscrowLocked},
			accountTypes:    []string{string(domain.AccountTypeLiability), string(domain.AccountTypeAsset)},
			journalContains: "Escrow order created rollback",
		},
		{
			name: "order released",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.RecordOrderReleased(context.Background(), domain.OrderReleasedWorkOrderUpdate{
					Payee:           "0x1111111111111111111111111111111111111111",
					Amount:          amount,
					OnchainOrderID:  orderID,
					TransactionHash: txHash,
					BlockNumber:     blockNumber,
					LogIndex:        logIndex,
					RecordedAt:      eventTime,
				})
			},
			journalKey:      "work_order:" + repositoryTestWorkOrderID.String() + ":order_released:" + eventID,
			ledgerTypes:     []string{string(domain.LedgerEntryTypeDebit), string(domain.LedgerEntryTypeCredit), string(domain.LedgerEntryTypeDebit), string(domain.LedgerEntryTypeCredit)},
			balanceDeltas:   []string{"-1000000", "-1000000", "1000000", "1000000"},
			accountNames:    []string{accountNameEscrowPending, accountNameEscrowLocked, accountNameServiceExpense, accountNameServiceRevenue},
			accountTypes:    []string{string(domain.AccountTypeLiability), string(domain.AccountTypeAsset), string(domain.AccountTypeExpense), string(domain.AccountTypeRevenue)},
			journalContains: "Escrow order released",
		},
		{
			name: "order released rollback",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.RollbackOrderReleased(context.Background(), domain.OrderReleasedWorkOrderRollback{
					Payee:           "0x1111111111111111111111111111111111111111",
					Amount:          amount,
					OnchainOrderID:  orderID,
					TransactionHash: txHash,
					BlockNumber:     blockNumber,
					LogIndex:        logIndex,
					RolledBackAt:    eventTime,
				})
			},
			journalKey:      "work_order:" + repositoryTestWorkOrderID.String() + ":order_released_rollback:" + eventID,
			ledgerTypes:     []string{string(domain.LedgerEntryTypeDebit), string(domain.LedgerEntryTypeCredit), string(domain.LedgerEntryTypeDebit), string(domain.LedgerEntryTypeCredit)},
			balanceDeltas:   []string{"1000000", "1000000", "-1000000", "-1000000"},
			accountNames:    []string{accountNameEscrowLocked, accountNameEscrowPending, accountNameServiceRevenue, accountNameServiceExpense},
			accountTypes:    []string{string(domain.AccountTypeAsset), string(domain.AccountTypeLiability), string(domain.AccountTypeRevenue), string(domain.AccountTypeExpense)},
			journalContains: "Escrow order released rollback",
		},
		{
			name: "order refunded",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.RecordOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderUpdate{
					Payer:           "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
					Amount:          amount,
					OnchainOrderID:  orderID,
					TransactionHash: txHash,
					BlockNumber:     blockNumber,
					LogIndex:        logIndex,
					RecordedAt:      eventTime,
				})
			},
			journalKey:      "work_order:" + repositoryTestWorkOrderID.String() + ":order_refunded:" + eventID,
			ledgerTypes:     []string{string(domain.LedgerEntryTypeDebit), string(domain.LedgerEntryTypeCredit)},
			balanceDeltas:   []string{"-1000000", "-1000000"},
			accountNames:    []string{accountNameEscrowPending, accountNameEscrowLocked},
			accountTypes:    []string{string(domain.AccountTypeLiability), string(domain.AccountTypeAsset)},
			journalContains: "Escrow order refunded",
		},
		{
			name: "order refunded rollback",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.RollbackOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderRollback{
					Payer:           "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
					Amount:          amount,
					OnchainOrderID:  orderID,
					TransactionHash: txHash,
					BlockNumber:     blockNumber,
					LogIndex:        logIndex,
					RolledBackAt:    eventTime,
				})
			},
			journalKey:      "work_order:" + repositoryTestWorkOrderID.String() + ":order_refunded_rollback:" + eventID,
			ledgerTypes:     []string{string(domain.LedgerEntryTypeDebit), string(domain.LedgerEntryTypeCredit)},
			balanceDeltas:   []string{"1000000", "1000000"},
			accountNames:    []string{accountNameEscrowLocked, accountNameEscrowPending},
			accountTypes:    []string{string(domain.AccountTypeAsset), string(domain.AccountTypeLiability)},
			journalContains: "Escrow order refunded rollback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
			storage := &fakeWorkOrderStorage{}
			repo := newEventWorkOrderRepositoryWithStorage(db, storage)

			updated, err := tt.run(repo)
			if err != nil {
				t.Fatalf("event method returned error: %v", err)
			}
			if !updated {
				t.Fatal("expected updated=true")
			}

			journals := db.operationsContaining("INSERT INTO journal_entries")
			if len(journals) != 1 {
				t.Fatalf("expected 1 journal write, got %d", len(journals))
			}
			assertArg(t, journals[0].args, 1, tt.journalKey)
			assertArg(t, journals[0].args, 2, repositoryTestWorkOrderID)
			if !strings.Contains(journals[0].args[3].(string), tt.journalContains) {
				t.Fatalf("expected journal description to contain %q, got %q", tt.journalContains, journals[0].args[3])
			}
			assertArg(t, journals[0].args, 4, repositoryTestStorageRootHash)
			assertArg(t, journals[0].args, 5, eventTime)

			if storage.calls != 1 {
				t.Fatalf("expected one storage upload, got %d", storage.calls)
			}
			payload, ok := storage.data.(bookkeepingJournalAnchor)
			if !ok {
				t.Fatalf("expected bookkeepingJournalAnchor payload, got %T", storage.data)
			}
			if payload.SchemaVersion != "1.0" || payload.Kind != "escrow_journal_entry" {
				t.Fatalf("unexpected payload metadata: %+v", payload)
			}
			if payload.JournalEntryID != journals[0].args[0].(uuid.UUID) ||
				payload.IdempotencyKey != tt.journalKey ||
				payload.WorkOrderID != repositoryTestWorkOrderID ||
				payload.CreatedAt != eventTime.UTC().Format(time.RFC3339Nano) ||
				!strings.Contains(payload.Description, tt.journalContains) {
				t.Fatalf("unexpected journal anchor payload: %+v", payload)
			}
			if len(payload.Postings) != len(tt.accountNames) {
				t.Fatalf("expected %d payload postings, got %d", len(tt.accountNames), len(payload.Postings))
			}
			for i := range payload.Postings {
				if payload.Postings[i].AccountName != tt.accountNames[i] ||
					string(payload.Postings[i].AccountType) != tt.accountTypes[i] ||
					string(payload.Postings[i].EntryType) != tt.ledgerTypes[i] ||
					payload.Postings[i].Amount != amount.String() {
					t.Fatalf("unexpected payload posting %d: %+v", i, payload.Postings[i])
				}
			}

			accountUpserts := db.operationsContaining("INSERT INTO accounts")
			if len(accountUpserts) != len(tt.accountNames) {
				t.Fatalf("expected %d account upserts, got %d", len(tt.accountNames), len(accountUpserts))
			}
			for i := range accountUpserts {
				if !strings.Contains(accountUpserts[i].sql, "ON CONFLICT (agent_id, name)") {
					t.Fatalf("expected idempotent account upsert, got %s", accountUpserts[i].sql)
				}
				assertArg(t, accountUpserts[i].args, 2, tt.accountNames[i])
				assertArg(t, accountUpserts[i].args, 3, tt.accountTypes[i])
				assertArg(t, accountUpserts[i].args, 4, eventTime)
			}

			ledgers := db.operationsContaining("INSERT INTO ledger_entries")
			if len(ledgers) != len(tt.ledgerTypes) {
				t.Fatalf("expected %d ledger writes, got %d", len(tt.ledgerTypes), len(ledgers))
			}
			for i := range ledgers {
				assertArg(t, ledgers[i].args, 3, amount.String())
				assertArg(t, ledgers[i].args, 4, tt.ledgerTypes[i])
				assertArg(t, ledgers[i].args, 5, eventTime)
			}

			balanceUpdates := db.operationsContaining("UPDATE accounts")
			if len(balanceUpdates) != len(tt.balanceDeltas) {
				t.Fatalf("expected %d account balance updates, got %d", len(tt.balanceDeltas), len(balanceUpdates))
			}
			for i := range balanceUpdates {
				assertArg(t, balanceUpdates[i].args, 0, tt.balanceDeltas[i])
			}

			if db.beginCalls != 1 || db.commitCalls != 1 || db.rollbackCalls != 0 {
				t.Fatalf("expected committed transaction, got begin=%d commit=%d rollback=%d", db.beginCalls, db.commitCalls, db.rollbackCalls)
			}
		})
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
