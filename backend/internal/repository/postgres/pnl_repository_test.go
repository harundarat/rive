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

var (
	pnlTestAgentID = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a20")
	pnlTestBatchID = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a30")
)

type fakePnLSQLOperation struct {
	sql  string
	args []any
}

type fakePnLDB struct {
	agentValues []any
	agentErr    error
	count       int64
	countErr    error
	accountRows pgx.Rows
	auditRows   pgx.Rows
	queryErr    error
	operations  []fakePnLSQLOperation
}

func (db *fakePnLDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.record(sql, args...)
	switch {
	case strings.Contains(sql, "FROM agents"):
		return fakePnLRow{values: db.agentValues, err: db.agentErr}
	case strings.Contains(sql, "COUNT(DISTINCT ledger_entries.journal_entry_id)"):
		return fakePnLRow{values: []any{db.count}, err: db.countErr}
	default:
		return fakePnLRow{err: errors.New("unexpected query row")}
	}
}

func (db *fakePnLDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.record(sql, args...)
	if db.queryErr != nil {
		return nil, db.queryErr
	}

	switch {
	case strings.Contains(sql, "GROUP BY accounts.type, accounts.name"):
		return db.accountRows, nil
	case strings.Contains(sql, "GROUP BY netting_batches.id"):
		return db.auditRows, nil
	default:
		return nil, errors.New("unexpected query")
	}
}

func (db *fakePnLDB) record(sql string, args ...any) {
	db.operations = append(db.operations, fakePnLSQLOperation{sql: sql, args: args})
}

type fakePnLRow struct {
	values []any
	err    error
}

func (row fakePnLRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return errors.New("unexpected scan destination count")
	}

	for i := range dest {
		if err := assignFakePnLValue(dest[i], row.values[i]); err != nil {
			return err
		}
	}

	return nil
}

type fakePnLRows struct {
	rows   [][]any
	index  int
	closed bool
	err    error
}

func (rows *fakePnLRows) Close() {
	rows.closed = true
}

func (rows *fakePnLRows) Err() error {
	return rows.err
}

func (rows *fakePnLRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("SELECT")
}

func (rows *fakePnLRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (rows *fakePnLRows) Next() bool {
	if rows.index >= len(rows.rows) {
		rows.Close()
		return false
	}

	rows.index++
	return true
}

func (rows *fakePnLRows) Scan(dest ...any) error {
	if rows.index == 0 || rows.index > len(rows.rows) {
		return errors.New("scan called without current row")
	}
	values := rows.rows[rows.index-1]
	if len(dest) != len(values) {
		return errors.New("unexpected scan destination count")
	}

	for i := range dest {
		if err := assignFakePnLValue(dest[i], values[i]); err != nil {
			return err
		}
	}

	return nil
}

func (rows *fakePnLRows) Values() ([]any, error) {
	if rows.index == 0 || rows.index > len(rows.rows) {
		return nil, errors.New("values called without current row")
	}

	return rows.rows[rows.index-1], nil
}

func (rows *fakePnLRows) RawValues() [][]byte {
	return nil
}

func (rows *fakePnLRows) Conn() *pgx.Conn {
	return nil
}

func assignFakePnLValue(dest any, value any) error {
	switch target := dest.(type) {
	case *uuid.UUID:
		*target = value.(uuid.UUID)
	case *string:
		*target = value.(string)
	case *int64:
		*target = value.(int64)
	case *time.Time:
		*target = value.(time.Time)
	case *pgtype.Text:
		if value == nil {
			*target = pgtype.Text{}
			return nil
		}
		*target = value.(pgtype.Text)
	case *pgtype.UUID:
		if value == nil {
			*target = pgtype.UUID{}
			return nil
		}
		*target = value.(pgtype.UUID)
	case *pgtype.Timestamptz:
		if value == nil {
			*target = pgtype.Timestamptz{}
			return nil
		}
		*target = value.(pgtype.Timestamptz)
	default:
		return errors.New("unsupported scan destination")
	}

	return nil
}

func TestPnLRepositoryGetPnLBuildsReportFromRevenueExpenseLedger(t *testing.T) {
	registeredAt := time.Date(2026, 4, 15, 8, 0, 0, 0, time.UTC)
	anchoredAt := time.Date(2026, 4, 22, 15, 30, 0, 0, time.UTC)
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	db := &fakePnLDB{
		agentValues: []any{pnlTestAgentID, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73", registeredAt},
		accountRows: &fakePnLRows{rows: [][]any{
			{string(domain.AccountTypeRevenue), "Service Revenue", "150000000", int64(15)},
			{string(domain.AccountTypeExpense), "Service Expense", "45000000", int64(5)},
		}},
		count: 20,
		auditRows: &fakePnLRows{rows: [][]any{
			{pnlTestBatchID.String(), pgtype.Text{String: "0xdef456", Valid: true}, int64(8), anchoredAt},
		}},
	}
	repo := &PnLRepository{db: db}

	report, err := repo.GetPnL(context.Background(), domain.PnLReportRequest{
		WalletAddress: "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		From:          &from,
		To:            to,
	})
	if err != nil {
		t.Fatalf("GetPnL returned error: %v", err)
	}

	if report.Agent.Address != "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73" {
		t.Fatalf("expected agent address, got %q", report.Agent.Address)
	}
	if report.Agent.RegisteredAt != "2026-04-15T08:00:00Z" {
		t.Fatalf("expected registeredAt, got %q", report.Agent.RegisteredAt)
	}
	if report.Summary.TotalRevenue != "150000000" ||
		report.Summary.TotalExpenses != "45000000" ||
		report.Summary.NetIncome != "105000000" ||
		report.Summary.TransactionCount != 20 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if len(report.Revenue) != 1 || report.Revenue[0].Account != "Service Revenue" || report.Revenue[0].EntryCount != 15 {
		t.Fatalf("unexpected revenue summary: %+v", report.Revenue)
	}
	if len(report.Expenses) != 1 || report.Expenses[0].Account != "Service Expense" || report.Expenses[0].EntryCount != 5 {
		t.Fatalf("unexpected expense summary: %+v", report.Expenses)
	}
	if report.AuditTrail.JournalBatchCount != 1 || len(report.AuditTrail.Batches) != 1 {
		t.Fatalf("unexpected audit trail: %+v", report.AuditTrail)
	}
	batch := report.AuditTrail.Batches[0]
	if batch.BatchID != pnlTestBatchID.String() || batch.StorageRootHash == nil || *batch.StorageRootHash != "0xdef456" {
		t.Fatalf("unexpected audit batch: %+v", batch)
	}
	if batch.ExplorerURL == nil || *batch.ExplorerURL != "https://storagescan.0g.ai/tx/0xdef456" {
		t.Fatalf("unexpected explorer URL: %v", batch.ExplorerURL)
	}

	if len(db.operations) != 4 {
		t.Fatalf("expected 4 SQL operations, got %d", len(db.operations))
	}
	assertSQLContains(t, db.operations[0].sql, "LOWER(wallet_address) = LOWER($1)")
	accountSQL := db.operations[1].sql
	for _, expected := range []string{
		"accounts.type IN ('revenue', 'expense')",
		"ledger_entries.created_at >= $2::timestamptz",
		"ledger_entries.created_at < $3::timestamptz",
		"WHEN accounts.type = 'revenue' AND ledger_entries.entry_type = 'credit' THEN ledger_entries.amount",
		"WHEN accounts.type = 'expense' AND ledger_entries.entry_type = 'credit' THEN -ledger_entries.amount",
	} {
		assertSQLContains(t, accountSQL, expected)
	}
	if db.operations[1].args[0] != pnlTestAgentID || db.operations[1].args[1] != from || db.operations[1].args[2] != to {
		t.Fatalf("unexpected account query args: %+v", db.operations[1].args)
	}
	assertSQLContains(t, db.operations[3].sql, "netting_batches.batch_status = 'settled'")
}

func TestPnLRepositoryGetPnLReturnsZeroReportForAgentWithoutPnLRows(t *testing.T) {
	registeredAt := time.Date(2026, 4, 15, 8, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	db := &fakePnLDB{
		agentValues: []any{pnlTestAgentID, "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73", registeredAt},
		accountRows: &fakePnLRows{},
		count:       0,
		auditRows:   &fakePnLRows{},
	}
	repo := &PnLRepository{db: db}

	report, err := repo.GetPnL(context.Background(), domain.PnLReportRequest{
		WalletAddress: "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		To:            to,
	})
	if err != nil {
		t.Fatalf("GetPnL returned error: %v", err)
	}
	if report.Summary.TotalRevenue != "0" || report.Summary.TotalExpenses != "0" || report.Summary.NetIncome != "0" {
		t.Fatalf("expected zero summary, got %+v", report.Summary)
	}
	if len(report.Revenue) != 0 || len(report.Expenses) != 0 || len(report.AuditTrail.Batches) != 0 {
		t.Fatalf("expected empty detail arrays, got revenue=%+v expenses=%+v audit=%+v", report.Revenue, report.Expenses, report.AuditTrail.Batches)
	}
	if db.operations[1].args[1] != nil {
		t.Fatalf("expected nil from arg, got %+v", db.operations[1].args[1])
	}
}

func TestPnLRepositoryGetPnLReturnsNotFoundForUnknownAgent(t *testing.T) {
	db := &fakePnLDB{agentErr: pgx.ErrNoRows}
	repo := &PnLRepository{db: db}

	_, err := repo.GetPnL(context.Background(), domain.PnLReportRequest{
		WalletAddress: "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		To:            time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if len(db.operations) != 1 {
		t.Fatalf("expected only agent query, got %d operations", len(db.operations))
	}
}

func assertSQLContains(t *testing.T, sql string, expected string) {
	t.Helper()

	if !strings.Contains(sql, expected) {
		t.Fatalf("expected SQL to contain %q, got %s", expected, sql)
	}
}
