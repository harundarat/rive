package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeNettingTx struct {
	rows       pgx.Rows
	queryErr   error
	execErr    error
	operations []fakePnLSQLOperation
	committed  bool
	rolledBack bool
}

func (tx *fakeNettingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	tx.record(sql, args...)
	if tx.queryErr != nil {
		return nil, tx.queryErr
	}

	return tx.rows, nil
}

func (tx *fakeNettingTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	tx.record(sql, args...)
	return fakePnLRow{err: errors.New("unexpected query row")}
}

func (tx *fakeNettingTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.record(sql, args...)
	if tx.execErr != nil {
		return pgconn.CommandTag{}, tx.execErr
	}

	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (tx *fakeNettingTx) Commit(ctx context.Context) error {
	tx.committed = true
	return nil
}

func (tx *fakeNettingTx) Rollback(ctx context.Context) error {
	tx.rolledBack = true
	return nil
}

func (tx *fakeNettingTx) record(sql string, args ...any) {
	tx.operations = append(tx.operations, fakePnLSQLOperation{sql: sql, args: args})
}

func TestNettingRepositoryClaimPendingIntentsLocksAndAssignsBatch(t *testing.T) {
	intentID := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a41")
	payerID := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a42")
	payeeID := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a43")
	batchID := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a44")
	createdAt := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	claimedAt := time.Date(2026, 5, 2, 10, 1, 0, 0, time.UTC)

	tx := &fakeNettingTx{
		rows: &fakePnLRows{rows: [][]any{
			{
				intentID,
				"intent-1",
				payerID,
				payeeID,
				"0x1111111111111111111111111111111111111111",
				"0x2222222222222222222222222222222222222222",
				"1000000",
				"rUSD",
				string(domain.PaymentIntentStatusPending),
				pgtype.UUID{},
				pgtype.Text{},
				createdAt,
				createdAt,
				pgtype.Timestamptz{},
			},
		}},
	}
	repo := &NettingRepository{
		beginTx: func(ctx context.Context) (nettingTx, error) {
			return tx, nil
		},
	}

	claim, err := repo.ClaimPendingIntents(context.Background(), claimedAt, batchID, claimedAt)
	if err != nil {
		t.Fatalf("ClaimPendingIntents returned error: %v", err)
	}

	if !tx.committed || tx.rolledBack {
		t.Fatalf("expected committed transaction, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
	if claim.Batch.ID != batchID || len(claim.Intents) != 1 || claim.Intents[0].ID != intentID {
		t.Fatalf("unexpected claim: %+v", claim)
	}
	if len(tx.operations) != 4 {
		t.Fatalf("expected select + insert + two updates, got %d operations", len(tx.operations))
	}

	selectSQL := tx.operations[0].sql
	for _, expected := range []string{
		"payment_intents.status = $1",
		"FOR UPDATE OF payment_intents SKIP LOCKED",
	} {
		if !strings.Contains(selectSQL, expected) {
			t.Fatalf("expected select SQL to contain %q, got %s", expected, selectSQL)
		}
	}
	if tx.operations[0].args[0] != string(domain.PaymentIntentStatusPending) {
		t.Fatalf("expected pending status arg, got %+v", tx.operations[0].args[0])
	}
	if !strings.Contains(tx.operations[2].sql, "status = $1") || !strings.Contains(tx.operations[2].sql, "netting_batch_id = $2") {
		t.Fatalf("expected payment intent batch assignment SQL, got %s", tx.operations[2].sql)
	}
	if !strings.Contains(tx.operations[3].sql, "WHERE payment_intent_id = ANY($2::uuid[])") {
		t.Fatalf("expected journal batch assignment SQL, got %s", tx.operations[3].sql)
	}
}
