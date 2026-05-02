package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/harundarat/rive/backend/internal/domain"
)

type PnLUsecase struct {
	pnlRepository domain.PnLRepository
	now           func() time.Time
}

func NewPnLUsecase(pnlRepository domain.PnLRepository) *PnLUsecase {
	return &PnLUsecase{
		pnlRepository: pnlRepository,
		now:           time.Now,
	}
}

func (uc *PnLUsecase) GetPnL(ctx context.Context, walletAddress string, fromRaw string, toRaw string) (*domain.PnLReport, error) {
	normalizedWallet, err := normalizePnLWalletAddress(walletAddress)
	if err != nil {
		return nil, err
	}

	from, err := parsePnLTimeParam("from", fromRaw)
	if err != nil {
		return nil, err
	}

	now := uc.now().UTC()
	to := now
	parsedTo, err := parsePnLTimeParam("to", toRaw)
	if err != nil {
		return nil, err
	}
	if parsedTo != nil {
		to = *parsedTo
	}

	if from != nil && !from.Before(to) {
		return nil, domain.NewValidationError("from must be before to")
	}

	report, err := uc.pnlRepository.GetPnL(ctx, domain.PnLReportRequest{
		WalletAddress: normalizedWallet,
		From:          from,
		To:            to,
	})
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: get pnl report: %w", domain.ErrPersistence, err)
	}
	if report == nil {
		return nil, fmt.Errorf("%w: get pnl report returned nil", domain.ErrPersistence)
	}

	report.Period = domain.PnLPeriod{
		From: formatOptionalPnLTime(from),
		To:   formatPnLTime(to),
	}
	report.Asset = domain.PnLAssetRUSD
	report.GeneratedAt = formatPnLTime(now)
	report.Version = domain.PnLVersion
	if report.Revenue == nil {
		report.Revenue = []domain.PnLAccountSummary{}
	}
	if report.Expenses == nil {
		report.Expenses = []domain.PnLAccountSummary{}
	}
	if report.AuditTrail.Batches == nil {
		report.AuditTrail.Batches = []domain.PnLAuditBatch{}
	}
	report.AuditTrail.JournalBatchCount = len(report.AuditTrail.Batches)

	return report, nil
}

func normalizePnLWalletAddress(walletAddress string) (string, error) {
	trimmed := strings.TrimSpace(walletAddress)
	if trimmed == "" {
		return "", domain.NewValidationError("walletAddress is required")
	}
	if !common.IsHexAddress(trimmed) {
		return "", domain.NewValidationError("walletAddress must be a valid Ethereum address")
	}

	return common.HexToAddress(trimmed).Hex(), nil
}

func parsePnLTimeParam(name string, value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, domain.NewValidationError("%s must be a valid RFC3339 timestamp", name)
	}
	parsed = parsed.UTC()

	return &parsed, nil
}

func formatOptionalPnLTime(value *time.Time) *string {
	if value == nil {
		return nil
	}

	formatted := formatPnLTime(*value)
	return &formatted
}

func formatPnLTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}
