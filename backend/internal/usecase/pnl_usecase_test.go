package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/harundarat/rive/backend/internal/domain"
)

var fixedPnLTime = time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)

type fakePnLRepository struct {
	request domain.PnLReportRequest
	report  *domain.PnLReport
	err     error
	calls   int
}

func (r *fakePnLRepository) GetPnL(ctx context.Context, request domain.PnLReportRequest) (*domain.PnLReport, error) {
	r.calls++
	r.request = request
	if r.err != nil {
		return nil, r.err
	}

	return r.report, nil
}

func TestPnLUsecaseGetPnLDefaultsPeriodAndMetadata(t *testing.T) {
	repo := &fakePnLRepository{
		report: &domain.PnLReport{
			Agent: domain.PnLAgent{
				Address:      "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
				RegisteredAt: "2026-04-15T08:00:00Z",
			},
			Summary: domain.PnLSummary{
				TotalRevenue:     "0",
				TotalExpenses:    "0",
				NetIncome:        "0",
				TransactionCount: 0,
			},
		},
	}
	uc := newTestPnLUsecase(repo)

	output, err := uc.GetPnL(context.Background(), "0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73", "", "")
	if err != nil {
		t.Fatalf("GetPnL returned error: %v", err)
	}
	if repo.calls != 1 {
		t.Fatalf("expected 1 repository call, got %d", repo.calls)
	}
	expectedWallet := common.HexToAddress("0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73").Hex()
	if repo.request.WalletAddress != expectedWallet {
		t.Fatalf("expected normalized wallet %q, got %q", expectedWallet, repo.request.WalletAddress)
	}
	if repo.request.From != nil {
		t.Fatalf("expected nil from, got %s", repo.request.From)
	}
	if !repo.request.To.Equal(fixedPnLTime) {
		t.Fatalf("expected to %s, got %s", fixedPnLTime, repo.request.To)
	}
	if output.Period.From != nil {
		t.Fatalf("expected response period from nil, got %v", output.Period.From)
	}
	if output.Period.To != "2026-05-01T10:30:00Z" {
		t.Fatalf("expected response period to, got %q", output.Period.To)
	}
	if output.GeneratedAt != "2026-05-01T10:30:00Z" {
		t.Fatalf("expected generatedAt, got %q", output.GeneratedAt)
	}
	if output.Asset != domain.PnLAssetRUSD || output.Decimals != domain.PnLAssetDecimals || output.Version != domain.PnLVersion {
		t.Fatalf("expected asset/decimals/version metadata, got %q/%d/%q", output.Asset, output.Decimals, output.Version)
	}
	if output.Revenue == nil || output.Expenses == nil || output.Transactions == nil || output.AuditTrail.Batches == nil {
		t.Fatal("expected non-nil response slices")
	}
}

func TestPnLUsecaseGetPnLParsesExplicitPeriodAsUTC(t *testing.T) {
	repo := &fakePnLRepository{report: &domain.PnLReport{}}
	uc := newTestPnLUsecase(repo)

	output, err := uc.GetPnL(
		context.Background(),
		"0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
		"2026-04-01T07:00:00+07:00",
		"2026-05-01T07:00:00+07:00",
	)
	if err != nil {
		t.Fatalf("GetPnL returned error: %v", err)
	}
	expectedFrom := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	expectedTo := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if repo.request.From == nil || !repo.request.From.Equal(expectedFrom) {
		t.Fatalf("expected from %s, got %v", expectedFrom, repo.request.From)
	}
	if !repo.request.To.Equal(expectedTo) {
		t.Fatalf("expected to %s, got %s", expectedTo, repo.request.To)
	}
	if output.Period.From == nil || *output.Period.From != "2026-04-01T00:00:00Z" {
		t.Fatalf("expected response from 2026-04-01T00:00:00Z, got %v", output.Period.From)
	}
	if output.Period.To != "2026-05-01T00:00:00Z" {
		t.Fatalf("expected response to, got %q", output.Period.To)
	}
}

func TestPnLUsecaseGetPnLValidation(t *testing.T) {
	tests := []struct {
		name    string
		wallet  string
		fromRaw string
		toRaw   string
	}{
		{name: "empty wallet", wallet: "", fromRaw: "", toRaw: ""},
		{name: "invalid wallet", wallet: "0xabc", fromRaw: "", toRaw: ""},
		{name: "invalid from", wallet: "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73", fromRaw: "yesterday", toRaw: ""},
		{name: "invalid to", wallet: "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73", fromRaw: "", toRaw: "tomorrow"},
		{name: "from equals to", wallet: "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73", fromRaw: "2026-05-01T00:00:00Z", toRaw: "2026-05-01T00:00:00Z"},
		{name: "from after to", wallet: "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73", fromRaw: "2026-05-02T00:00:00Z", toRaw: "2026-05-01T00:00:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakePnLRepository{report: &domain.PnLReport{}}
			uc := newTestPnLUsecase(repo)

			_, err := uc.GetPnL(context.Background(), tt.wallet, tt.fromRaw, tt.toRaw)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var validationErr *domain.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %T", err)
			}
			if repo.calls != 0 {
				t.Fatalf("expected no repository call, got %d", repo.calls)
			}
		})
	}
}

func TestPnLUsecaseGetPnLErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		repoErr    error
		expectedIs error
	}{
		{name: "not found", repoErr: domain.ErrNotFound, expectedIs: domain.ErrNotFound},
		{name: "persistence", repoErr: errors.New("database unavailable"), expectedIs: domain.ErrPersistence},
		{name: "nil report", repoErr: nil, expectedIs: domain.ErrPersistence},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakePnLRepository{err: tt.repoErr}
			if tt.name != "nil report" {
				repo.report = &domain.PnLReport{}
			}
			uc := newTestPnLUsecase(repo)

			_, err := uc.GetPnL(context.Background(), "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73", "", "")
			if !errors.Is(err, tt.expectedIs) {
				t.Fatalf("expected error %v, got %v", tt.expectedIs, err)
			}
		})
	}
}

func newTestPnLUsecase(repo *fakePnLRepository) *PnLUsecase {
	uc := NewPnLUsecase(repo)
	uc.now = func() time.Time { return fixedPnLTime }

	return uc
}
