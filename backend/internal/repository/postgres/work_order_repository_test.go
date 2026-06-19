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
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	repositoryTestWorkOrderID = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a1f")
	repositoryTestPayerID     = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a20")
	repositoryTestPayeeID     = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a21")
	repositoryTestJournalID   = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a22")
)

const repositoryTestStorageRootHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeSQLOperation struct {
	sql  string
	args []any
}

type fakeWorkOrderDB struct {
	sql           string
	args          []any
	rowsAffected  int64
	err           error
	target        *domain.WorkOrderBookkeepingTarget
	operations    []fakeSQLOperation
	beginCalls    int
	commitCalls   int
	rollbackCalls int
	accountIDs    map[string]uuid.UUID
}

func (db *fakeWorkOrderDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.record(sql, args...)
	if db.err != nil {
		return fakeWorkOrderRow{err: db.err}
	}
	if strings.Contains(sql, "SELECT work_orders.id, work_orders.creator_id, work_orders.provider_id") {
		if db.target == nil {
			return fakeWorkOrderRow{err: pgx.ErrNoRows}
		}

		return fakeWorkOrderRow{values: []any{db.target.WorkOrderID, db.target.PayerID, db.target.PayeeID}}
	}

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

	if strings.Contains(sql, "INSERT INTO accounts") {
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
	}

	return fakeWorkOrderRow{err: errors.New("unexpected transaction query row scan")}
}

func (tx *fakeWorkOrderTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.db.record(sql, args...)
	if tx.db.err != nil {
		return pgconn.CommandTag{}, tx.db.err
	}

	if strings.Contains(sql, "UPDATE work_orders") {
		return pgconn.NewCommandTag("UPDATE " + big.NewInt(tx.db.rowsAffected).String()), nil
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
		case *string:
			*value = row.values[i].(string)
		case *pgtype.Text:
			*value = row.values[i].(pgtype.Text)
		case *pgtype.Timestamptz:
			*value = row.values[i].(pgtype.Timestamptz)
		case *time.Time:
			*value = row.values[i].(time.Time)
		default:
			return errors.New("unsupported scan destination")
		}
	}

	return nil
}

func newEventWorkOrderRepository(db *fakeWorkOrderDB) *WorkOrderRepository {
	return &WorkOrderRepository{
		db:      db,
		beginTx: db.beginTx,
		newID:   uuid.NewV7,
	}
}

func validBookkeepingTarget() *domain.WorkOrderBookkeepingTarget {
	return &domain.WorkOrderBookkeepingTarget{
		WorkOrderID: repositoryTestWorkOrderID,
		PayerID:     repositoryTestPayerID,
		PayeeID:     repositoryTestPayeeID,
	}
}

func sampleEscrowEntry() domain.EscrowJournalEntry {
	return domain.EscrowJournalEntry{
		JournalID:      repositoryTestJournalID,
		IdempotencyKey: "work_order:" + repositoryTestWorkOrderID.String() + ":order_created:0xtx",
		WorkOrderID:    repositoryTestWorkOrderID,
		Description:    "Escrow order created for on-chain order 123",
		StorageCID:     repositoryTestStorageRootHash,
		CreatedAt:      time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
		Amount:         bigIntFromStringForRepositoryTest("1000000"),
		Postings: []domain.LedgerPosting{
			{AgentID: repositoryTestPayerID, AccountName: "escrow_locked", AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeDebit},
			{AgentID: repositoryTestPayeeID, AccountName: "escrow_pending", AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeCredit},
		},
	}
}

// Resolve* methods must be read-only: a single SELECT, no transaction, and never
// any 0G upload (the repository has no storage uploader at all).
func TestWorkOrderRepositoryResolveTargets(t *testing.T) {
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	amount := bigIntFromStringForRepositoryTest("1000000")
	specHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	txHash := "0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70"
	payer := "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73"
	payee := "0x1111111111111111111111111111111111111111"

	tests := []struct {
		name         string
		run          func(*WorkOrderRepository) (*domain.WorkOrderBookkeepingTarget, bool, error)
		sqlFragments []string
		args         map[int]any
	}{
		{
			name: "order created",
			run: func(repo *WorkOrderRepository) (*domain.WorkOrderBookkeepingTarget, bool, error) {
				return repo.ResolveOrderCreatedTarget(context.Background(), domain.OrderCreatedWorkOrderUpdate{
					SpecHash: specHash, Payer: payer, Payee: payee, Amount: amount, OnchainOrderID: largeOrderID, TransactionHash: txHash,
				})
			},
			sqlFragments: []string{
				"SELECT work_orders.id, work_orders.creator_id, work_orders.provider_id",
				"work_orders.spec_hash = $1",
				"work_orders.amount = $2::numeric",
				"work_orders.status = $3",
				"LOWER(payer.wallet_address) = LOWER($4)",
				"LOWER(payee.wallet_address) = LOWER($5)",
			},
			args: map[int]any{0: specHash, 1: amount.String(), 2: string(domain.WorkOrderStatusDraft), 3: payer, 4: payee},
		},
		{
			name: "order created rollback",
			run: func(repo *WorkOrderRepository) (*domain.WorkOrderBookkeepingTarget, bool, error) {
				return repo.ResolveOrderCreatedRollbackTarget(context.Background(), domain.OrderCreatedWorkOrderRollback{
					SpecHash: specHash, Payer: payer, Payee: payee, Amount: amount, OnchainOrderID: largeOrderID, TransactionHash: txHash,
				})
			},
			sqlFragments: []string{
				"work_orders.spec_hash = $1",
				"work_orders.onchain_order_id = $3::numeric",
				"work_orders.order_tx_hash = $4",
				"work_orders.status = $5",
				"LOWER(payer.wallet_address) = LOWER($6)",
				"LOWER(payee.wallet_address) = LOWER($7)",
			},
			args: map[int]any{0: specHash, 1: amount.String(), 2: largeOrderID.String(), 3: txHash, 4: string(domain.WorkOrderStatusFunded), 5: payer, 6: payee},
		},
		{
			name: "order released",
			run: func(repo *WorkOrderRepository) (*domain.WorkOrderBookkeepingTarget, bool, error) {
				return repo.ResolveOrderReleasedTarget(context.Background(), domain.OrderReleasedWorkOrderUpdate{
					Payee: payee, Amount: amount, OnchainOrderID: largeOrderID,
				})
			},
			sqlFragments: []string{
				"work_orders.onchain_order_id = $1::numeric",
				"work_orders.amount = $2::numeric",
				"work_orders.status = $3",
				"LOWER(payee.wallet_address) = LOWER($4)",
			},
			args: map[int]any{0: largeOrderID.String(), 1: amount.String(), 2: string(domain.WorkOrderStatusFunded), 3: payee},
		},
		{
			name: "order released rollback",
			run: func(repo *WorkOrderRepository) (*domain.WorkOrderBookkeepingTarget, bool, error) {
				return repo.ResolveOrderReleasedRollbackTarget(context.Background(), domain.OrderReleasedWorkOrderRollback{
					Payee: payee, Amount: amount, OnchainOrderID: largeOrderID,
				})
			},
			sqlFragments: []string{
				"work_orders.onchain_order_id = $1::numeric",
				"work_orders.status = $3",
				"LOWER(payee.wallet_address) = LOWER($4)",
			},
			args: map[int]any{0: largeOrderID.String(), 1: amount.String(), 2: string(domain.WorkOrderStatusCompleted), 3: payee},
		},
		{
			name: "order refunded",
			run: func(repo *WorkOrderRepository) (*domain.WorkOrderBookkeepingTarget, bool, error) {
				return repo.ResolveOrderRefundedTarget(context.Background(), domain.OrderRefundedWorkOrderUpdate{
					Payer: payer, Amount: amount, OnchainOrderID: largeOrderID,
				})
			},
			sqlFragments: []string{
				"work_orders.onchain_order_id = $1::numeric",
				"work_orders.status = $3",
				"LOWER(payer.wallet_address) = LOWER($4)",
			},
			args: map[int]any{0: largeOrderID.String(), 1: amount.String(), 2: string(domain.WorkOrderStatusFunded), 3: payer},
		},
		{
			name: "order refunded rollback",
			run: func(repo *WorkOrderRepository) (*domain.WorkOrderBookkeepingTarget, bool, error) {
				return repo.ResolveOrderRefundedRollbackTarget(context.Background(), domain.OrderRefundedWorkOrderRollback{
					Payer: payer, Amount: amount, OnchainOrderID: largeOrderID,
				})
			},
			sqlFragments: []string{
				"work_orders.onchain_order_id = $1::numeric",
				"work_orders.status = $3",
				"LOWER(payer.wallet_address) = LOWER($4)",
			},
			args: map[int]any{0: largeOrderID.String(), 1: amount.String(), 2: string(domain.WorkOrderStatusRefunded), 3: payer},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeWorkOrderDB{target: validBookkeepingTarget()}
			repo := newEventWorkOrderRepository(db)

			target, matched, err := tt.run(repo)
			if err != nil {
				t.Fatalf("resolve returned error: %v", err)
			}
			if !matched {
				t.Fatal("expected matched=true")
			}
			if target.WorkOrderID != repositoryTestWorkOrderID || target.PayerID != repositoryTestPayerID || target.PayeeID != repositoryTestPayeeID {
				t.Fatalf("unexpected target: %+v", target)
			}

			op := db.operation(t, 0)
			for _, fragment := range tt.sqlFragments {
				if !strings.Contains(op.sql, fragment) {
					t.Fatalf("expected SQL to contain %q, got %s", fragment, op.sql)
				}
			}
			for index, expected := range tt.args {
				assertArg(t, op.args, index, expected)
			}

			if db.beginCalls != 0 || db.containsSQL("UPDATE work_orders") || db.containsSQL("INSERT INTO journal_entries") {
				t.Fatalf("resolve must be read-only, got operations %+v", db.operations)
			}
		})
	}
}

func TestWorkOrderRepositoryResolveReturnsFalseWhenNoMatch(t *testing.T) {
	db := &fakeWorkOrderDB{target: nil}
	repo := newEventWorkOrderRepository(db)

	target, matched, err := repo.ResolveOrderCreatedTarget(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		SpecHash: "0xspec", Payer: "0xpayer", Payee: "0xpayee", Amount: bigIntFromStringForRepositoryTest("1"),
	})
	if err != nil {
		t.Fatalf("resolve returned error: %v", err)
	}
	if matched || target != nil {
		t.Fatalf("expected no match, got matched=%v target=%+v", matched, target)
	}
}

// Apply* methods perform the guarded state transition and the prepared journal
// writes inside a single transaction, with the StorageCID already supplied — so no
// network call is held open inside the tx.
func TestWorkOrderRepositoryApplyTransitions(t *testing.T) {
	largeOrderID := bigIntFromStringForRepositoryTest("1606938044258990275541962092341162602522202993782792835301376")
	amount := bigIntFromStringForRepositoryTest("1000000")
	txHash := "0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70"
	recordedAt := time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		run          func(*WorkOrderRepository) (bool, error)
		sqlFragments []string
		args         map[int]any
	}{
		{
			name: "order created",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.ApplyOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
					OnchainOrderID: largeOrderID, TransactionHash: txHash, Amount: amount, RecordedAt: recordedAt,
				}, sampleEscrowEntry())
			},
			sqlFragments: []string{"status = $1", "onchain_order_id = $2::numeric", "order_tx_hash = $3", "updated_at = $4", "WHERE id = $5", "AND status = $6"},
			args:         map[int]any{0: string(domain.WorkOrderStatusFunded), 1: largeOrderID.String(), 2: txHash, 3: recordedAt, 4: repositoryTestWorkOrderID, 5: string(domain.WorkOrderStatusDraft)},
		},
		{
			name: "order created rollback",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.ApplyOrderCreatedRollback(context.Background(), domain.OrderCreatedWorkOrderRollback{
					OnchainOrderID: largeOrderID, TransactionHash: txHash, Amount: amount, RolledBackAt: recordedAt,
				}, sampleEscrowEntry())
			},
			sqlFragments: []string{"status = $1", "onchain_order_id = NULL", "order_tx_hash = NULL", "updated_at = $2", "WHERE id = $3", "AND status = $4"},
			args:         map[int]any{0: string(domain.WorkOrderStatusDraft), 1: recordedAt, 2: repositoryTestWorkOrderID, 3: string(domain.WorkOrderStatusFunded)},
		},
		{
			name: "order released",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.ApplyOrderReleased(context.Background(), domain.OrderReleasedWorkOrderUpdate{
					OnchainOrderID: largeOrderID, TransactionHash: txHash, Amount: amount, RecordedAt: recordedAt,
				}, sampleEscrowEntry())
			},
			sqlFragments: []string{"status = $1", "completed_at = $2", "release_tx_hash = $3", "updated_at = $2", "WHERE id = $4", "AND status = $5"},
			args:         map[int]any{0: string(domain.WorkOrderStatusCompleted), 1: recordedAt, 2: txHash, 3: repositoryTestWorkOrderID, 4: string(domain.WorkOrderStatusFunded)},
		},
		{
			name: "order released rollback",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.ApplyOrderReleasedRollback(context.Background(), domain.OrderReleasedWorkOrderRollback{
					OnchainOrderID: largeOrderID, TransactionHash: txHash, Amount: amount, RolledBackAt: recordedAt,
				}, sampleEscrowEntry())
			},
			sqlFragments: []string{"status = $1", "completed_at = NULL", "release_tx_hash = NULL", "updated_at = $2", "WHERE id = $3", "AND status = $4"},
			args:         map[int]any{0: string(domain.WorkOrderStatusFunded), 1: recordedAt, 2: repositoryTestWorkOrderID, 3: string(domain.WorkOrderStatusCompleted)},
		},
		{
			name: "order refunded",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.ApplyOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderUpdate{
					OnchainOrderID: largeOrderID, TransactionHash: txHash, Amount: amount, RecordedAt: recordedAt,
				}, sampleEscrowEntry())
			},
			sqlFragments: []string{"status = $1", "refunded_at = $2", "refund_tx_hash = $3", "updated_at = $2", "WHERE id = $4", "AND status = $5"},
			args:         map[int]any{0: string(domain.WorkOrderStatusRefunded), 1: recordedAt, 2: txHash, 3: repositoryTestWorkOrderID, 4: string(domain.WorkOrderStatusFunded)},
		},
		{
			name: "order refunded rollback",
			run: func(repo *WorkOrderRepository) (bool, error) {
				return repo.ApplyOrderRefundedRollback(context.Background(), domain.OrderRefundedWorkOrderRollback{
					OnchainOrderID: largeOrderID, TransactionHash: txHash, Amount: amount, RolledBackAt: recordedAt,
				}, sampleEscrowEntry())
			},
			sqlFragments: []string{"status = $1", "refunded_at = NULL", "refund_tx_hash = NULL", "updated_at = $2", "WHERE id = $3", "AND status = $4"},
			args:         map[int]any{0: string(domain.WorkOrderStatusFunded), 1: recordedAt, 2: repositoryTestWorkOrderID, 3: string(domain.WorkOrderStatusRefunded)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeWorkOrderDB{rowsAffected: 1}
			repo := newEventWorkOrderRepository(db)

			updated, err := tt.run(repo)
			if err != nil {
				t.Fatalf("apply returned error: %v", err)
			}
			if !updated {
				t.Fatal("expected updated=true")
			}

			updateOp := db.operation(t, 0)
			if !strings.Contains(updateOp.sql, "UPDATE work_orders") {
				t.Fatalf("expected first operation to be UPDATE work_orders, got %s", updateOp.sql)
			}
			for _, fragment := range tt.sqlFragments {
				if !strings.Contains(updateOp.sql, fragment) {
					t.Fatalf("expected SQL to contain %q, got %s", fragment, updateOp.sql)
				}
			}
			for index, expected := range tt.args {
				assertArg(t, updateOp.args, index, expected)
			}

			if !db.containsSQL("INSERT INTO journal_entries") {
				t.Fatal("expected journal write after a matched transition")
			}
			if db.beginCalls != 1 || db.commitCalls != 1 || db.rollbackCalls != 0 {
				t.Fatalf("expected committed transaction, got begin=%d commit=%d rollback=%d", db.beginCalls, db.commitCalls, db.rollbackCalls)
			}
		})
	}
}

func TestWorkOrderRepositoryApplyPersistsJournalAndLedger(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 1}
	repo := newEventWorkOrderRepository(db)
	entry := sampleEscrowEntry()

	updated, err := repo.ApplyOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		OnchainOrderID:  bigIntFromStringForRepositoryTest("123"),
		TransactionHash: "0xtx",
		Amount:          entry.Amount,
		RecordedAt:      entry.CreatedAt,
	}, entry)
	if err != nil {
		t.Fatalf("ApplyOrderCreated returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	journals := db.operationsContaining("INSERT INTO journal_entries")
	if len(journals) != 1 {
		t.Fatalf("expected 1 journal write, got %d", len(journals))
	}
	assertArg(t, journals[0].args, 0, entry.JournalID)
	assertArg(t, journals[0].args, 1, entry.IdempotencyKey)
	assertArg(t, journals[0].args, 2, entry.WorkOrderID)
	assertArg(t, journals[0].args, 3, entry.Description)
	assertArg(t, journals[0].args, 4, entry.StorageCID)
	assertArg(t, journals[0].args, 5, entry.CreatedAt)

	accountUpserts := db.operationsContaining("INSERT INTO accounts")
	if len(accountUpserts) != len(entry.Postings) {
		t.Fatalf("expected %d account upserts, got %d", len(entry.Postings), len(accountUpserts))
	}
	for i, posting := range entry.Postings {
		if !strings.Contains(accountUpserts[i].sql, "ON CONFLICT (agent_id, name)") {
			t.Fatalf("expected idempotent account upsert, got %s", accountUpserts[i].sql)
		}
		assertArg(t, accountUpserts[i].args, 1, posting.AgentID)
		assertArg(t, accountUpserts[i].args, 2, posting.AccountName)
		assertArg(t, accountUpserts[i].args, 3, string(posting.AccountType))
	}

	ledgers := db.operationsContaining("INSERT INTO ledger_entries")
	if len(ledgers) != len(entry.Postings) {
		t.Fatalf("expected %d ledger writes, got %d", len(entry.Postings), len(ledgers))
	}
	for i, posting := range entry.Postings {
		assertArg(t, ledgers[i].args, 2, entry.JournalID)
		assertArg(t, ledgers[i].args, 3, entry.Amount.String())
		assertArg(t, ledgers[i].args, 4, string(posting.EntryType))
		assertArg(t, ledgers[i].args, 5, entry.CreatedAt)
	}

	balanceUpdates := db.operationsContaining("UPDATE accounts")
	if len(balanceUpdates) != len(entry.Postings) {
		t.Fatalf("expected %d balance updates, got %d", len(entry.Postings), len(balanceUpdates))
	}
	// escrow_locked (asset/debit) and escrow_pending (liability/credit) both add +amount.
	for i := range entry.Postings {
		assertArg(t, balanceUpdates[i].args, 0, entry.Amount.String())
	}

	if db.beginCalls != 1 || db.commitCalls != 1 || db.rollbackCalls != 0 {
		t.Fatalf("expected committed transaction, got begin=%d commit=%d rollback=%d", db.beginCalls, db.commitCalls, db.rollbackCalls)
	}
}

func TestWorkOrderRepositoryApplyIsNoopWhenTransitionMatchesNoRow(t *testing.T) {
	db := &fakeWorkOrderDB{rowsAffected: 0}
	repo := newEventWorkOrderRepository(db)

	updated, err := repo.ApplyOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		OnchainOrderID:  bigIntFromStringForRepositoryTest("123"),
		TransactionHash: "0xtx",
		Amount:          bigIntFromStringForRepositoryTest("1000000"),
		RecordedAt:      time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
	}, sampleEscrowEntry())
	if err != nil {
		t.Fatalf("ApplyOrderCreated returned error: %v", err)
	}
	if updated {
		t.Fatal("expected updated=false when the guarded transition matches no row")
	}
	if db.containsSQL("INSERT INTO journal_entries") || db.containsSQL("INSERT INTO ledger_entries") || db.containsSQL("INSERT INTO accounts") {
		t.Fatalf("expected no bookkeeping writes for a no-op transition, got %+v", db.operations)
	}
	if db.beginCalls != 1 || db.commitCalls != 1 || db.rollbackCalls != 0 {
		t.Fatalf("expected clean commit for no-op, got begin=%d commit=%d rollback=%d", db.beginCalls, db.commitCalls, db.rollbackCalls)
	}
}

// release_tx_hash/refund_tx_hash are written by the release/refund transitions and
// read by the PnL query; this guards that the work_orders projection also surfaces
// them on domain.WorkOrder (the single-source column list + scan must stay in sync).
func TestWorkOrderRepositoryScanSurfacesReleaseAndRefundTxHash(t *testing.T) {
	releaseTx := "0x" + strings.Repeat("a", 64)
	refundTx := "0x" + strings.Repeat("b", 64)
	createdAt := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	row := fakeWorkOrderRow{values: []any{
		repositoryTestWorkOrderID,               // id
		"work_order:idempotency",                // idempotency_key
		repositoryTestPayerID,                   // creator_id
		repositoryTestPayeeID,                   // provider_id
		"1000000",                               // amount::text
		string(domain.WorkOrderStatusCompleted), // status
		"0xspec",                                // spec_hash
		"1.0",                                   // spec_version
		"0xspectx",                              // spec_tx_hash
		pgtype.Text{},                           // deliverable_cid (NULL)
		pgtype.Timestamptz{},                    // delivered_at (NULL)
		pgtype.Timestamptz{},                    // completed_at (NULL)
		pgtype.Timestamptz{},                    // refunded_at (NULL)
		pgtype.Text{},                           // onchain_order_id::text (NULL)
		pgtype.Text{},                           // order_tx_hash (NULL)
		pgtype.Text{String: releaseTx, Valid: true}, // release_tx_hash
		pgtype.Text{String: refundTx, Valid: true},  // refund_tx_hash
		createdAt, // created_at
		createdAt, // updated_at
	}}

	workOrder, err := scanWorkOrderWithExtra(row)
	if err != nil {
		t.Fatalf("scanWorkOrderWithExtra returned error: %v", err)
	}
	if workOrder.ReleaseTxHash == nil || *workOrder.ReleaseTxHash != releaseTx {
		t.Fatalf("expected release_tx_hash %q, got %v", releaseTx, workOrder.ReleaseTxHash)
	}
	if workOrder.RefundTxHash == nil || *workOrder.RefundTxHash != refundTx {
		t.Fatalf("expected refund_tx_hash %q, got %v", refundTx, workOrder.RefundTxHash)
	}
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
