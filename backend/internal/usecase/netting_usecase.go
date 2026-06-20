package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

const (
	nettingAssetRUSD       = "rUSD"
	nettingManifestVersion = "1.0"

	// defaultSettleTimeout bounds the on-chain settle step so a transaction that
	// never mines (bind.WaitMined) cannot wedge the whole netting loop.
	defaultSettleTimeout = 90 * time.Second
	// reconcileStuckReason labels batches the startup reconcile marks failed.
	reconcileStuckReason = "reconciliation: batch left in processing across restart"
)

type NettingUsecase struct {
	storage           domain.Storage
	nettingRepository domain.NettingRepository
	agentRepository   domain.AgentRepository
	settlementGateway domain.NettingSettlementGateway
	window            time.Duration
	settleTimeout     time.Duration
	reconcileGrace    time.Duration
	now               func() time.Time
	newID             func() (uuid.UUID, error)
}

func NewNettingUsecase(
	storage domain.Storage,
	nettingRepository domain.NettingRepository,
	agentRepository domain.AgentRepository,
	settlementGateway domain.NettingSettlementGateway,
	window time.Duration,
) *NettingUsecase {
	if window <= 0 {
		window = time.Minute
	}

	// reconcileGrace must exceed a normal flush (which includes WaitMined) so a
	// batch claimed by a live worker is never clobbered as "stuck".
	reconcileGrace := 2 * window
	if reconcileGrace < time.Minute {
		reconcileGrace = time.Minute
	}

	return &NettingUsecase{
		storage:           storage,
		nettingRepository: nettingRepository,
		agentRepository:   agentRepository,
		settlementGateway: settlementGateway,
		window:            window,
		settleTimeout:     defaultSettleTimeout,
		reconcileGrace:    reconcileGrace,
		now:               time.Now,
		newID:             uuid.NewV7,
	}
}

// ReconcileStuckBatches fails any batch still in processing past reconcileGrace —
// an orphan from a crash between claim and settlement. It is idempotent (a failed
// batch is no longer processing) and best-effort per batch, so one failure does
// not abort the rest. Run once at startup before the netting loop begins.
func (uc *NettingUsecase) ReconcileStuckBatches(ctx context.Context) error {
	cutoff := uc.now().UTC().Add(-uc.reconcileGrace)
	batchIDs, err := uc.nettingRepository.FindStuckProcessingBatches(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("%w: reconcile stuck netting batches: %w", domain.ErrPersistence, err)
	}

	for _, batchID := range batchIDs {
		if err := uc.nettingRepository.MarkBatchFailed(ctx, batchID, reconcileStuckReason, uc.now().UTC()); err != nil {
			slog.Error("reconcile: mark stuck netting batch failed", "batch_id", batchID, "error", err)
			continue
		}
	}
	if len(batchIDs) > 0 {
		slog.Warn("reconciled stuck netting batches", "count", len(batchIDs))
	}

	return nil
}

func (uc *NettingUsecase) SubmitIntent(ctx context.Context, request domain.PaymentIntentRequest) (*domain.PaymentIntentResponse, error) {
	if isBlank(request.IdempotencyKey) {
		return nil, domain.NewValidationError("idempotency_key is required")
	}

	existing, err := uc.nettingRepository.FindIntentByIdempotencyKey(ctx, request.IdempotencyKey)
	if err == nil {
		return paymentIntentResponse(existing), nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%w: find existing payment intent: %w", domain.ErrPersistence, err)
	}

	normalized, err := validatePaymentIntentRequest(request)
	if err != nil {
		return nil, err
	}

	payer, err := uc.findNettingAgentByWallet(ctx, "payer", normalized.Payer)
	if err != nil {
		return nil, err
	}
	payee, err := uc.findNettingAgentByWallet(ctx, "payee", normalized.Payee)
	if err != nil {
		return nil, err
	}

	id, err := uc.newID()
	if err != nil {
		return nil, fmt.Errorf("generate payment intent id: %w", err)
	}
	now := uc.now().UTC()
	intent := domain.PaymentIntent{
		ID:             id,
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		PayerID:        payer.ID,
		PayeeID:        payee.ID,
		PayerWallet:    payer.WalletAddress,
		PayeeWallet:    payee.WalletAddress,
		Amount:         normalized.Amount,
		Asset:          nettingAssetRUSD,
		Status:         domain.PaymentIntentStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	created, err := uc.nettingRepository.CreateIntentWithAccrual(ctx, intent)
	if err != nil {
		existing, findErr := uc.nettingRepository.FindIntentByIdempotencyKey(ctx, request.IdempotencyKey)
		if findErr == nil {
			return paymentIntentResponse(existing), nil
		}

		return nil, fmt.Errorf("%w: create payment intent: %w", domain.ErrPersistence, err)
	}

	return paymentIntentResponse(created), nil
}

func (uc *NettingUsecase) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(uc.window)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := uc.FlushPending(ctx); err != nil {
					slog.Error("netting flush failed", "error", err)
				}
			}
		}
	}()
}

func (uc *NettingUsecase) FlushPending(ctx context.Context) (*domain.NettingBatchSettlement, error) {
	batchID, err := uc.newID()
	if err != nil {
		return nil, fmt.Errorf("generate netting batch id: %w", err)
	}
	now := uc.now().UTC()

	claim, err := uc.nettingRepository.ClaimPendingIntents(ctx, now, batchID, now)
	if err != nil {
		return nil, fmt.Errorf("%w: claim pending payment intents: %w", domain.ErrPersistence, err)
	}
	if claim == nil || len(claim.Intents) == 0 {
		return nil, nil
	}

	summary, err := computeNettingSummary(claim.Batch.ID, claim.Intents, now)
	if err != nil {
		_ = uc.nettingRepository.MarkBatchFailed(ctx, claim.Batch.ID, err.Error(), uc.now().UTC())
		return nil, err
	}

	uploadOutput, err := uc.storage.UploadJSON(ctx, summary.Manifest)
	if err != nil {
		_ = uc.nettingRepository.MarkBatchFailed(ctx, claim.Batch.ID, err.Error(), uc.now().UTC())
		return nil, fmt.Errorf("%w: upload netting batch manifest: %w", domain.ErrStorage, err)
	}
	summary.Settlement.BatchHash = uploadOutput.RootHash
	summary.Settlement.ManifestTxHash = uploadOutput.TxHash

	instruction := domain.NettingSettlementInstruction{
		BatchHash:     uploadOutput.RootHash,
		Debtors:       summary.Settlement.Debtors,
		Creditors:     summary.Settlement.Creditors,
		SkipOnchainTx: summary.Settlement.NetAmount.Sign() == 0,
	}
	settleCtx, cancel := context.WithTimeout(ctx, uc.settleTimeout)
	receipt, err := uc.settlementGateway.SettleBatch(settleCtx, instruction)
	cancel()
	if err != nil {
		// Cleanup uses the outer ctx (still live) so a settle-step timeout can
		// still mark the batch failed and let the loop proceed.
		_ = uc.nettingRepository.MarkBatchFailed(ctx, claim.Batch.ID, err.Error(), uc.now().UTC())
		return nil, fmt.Errorf("%w: settle netting batch: %w", domain.ErrSettlement, err)
	}
	if receipt != nil {
		summary.Settlement.SettlementTxHash = receipt.TransactionHash
	}

	if err := uc.nettingRepository.MarkBatchSettled(ctx, summary.Settlement); err != nil {
		return nil, fmt.Errorf("%w: mark netting batch settled: %w", domain.ErrPersistence, err)
	}

	return &summary.Settlement, nil
}

type normalizedPaymentIntentRequest struct {
	Payer  string
	Payee  string
	Amount big.Int
}

func validatePaymentIntentRequest(request domain.PaymentIntentRequest) (*normalizedPaymentIntentRequest, error) {
	payer, err := normalizeNettingWalletAddress("payer", request.Payer)
	if err != nil {
		return nil, err
	}
	payee, err := normalizeNettingWalletAddress("payee", request.Payee)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(payer, payee) {
		return nil, domain.NewValidationError("payer and payee must be different")
	}

	asset := strings.TrimSpace(request.Asset)
	if !strings.EqualFold(asset, nettingAssetRUSD) {
		return nil, fmt.Errorf("%w: asset must be rUSD", domain.ErrUnsupportedAsset)
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(strings.TrimSpace(request.Amount), 10); !ok || amount.Sign() <= 0 {
		return nil, domain.NewValidationError("amount must be a positive integer")
	}

	return &normalizedPaymentIntentRequest{Payer: payer, Payee: payee, Amount: *amount}, nil
}

func normalizeNettingWalletAddress(fieldName string, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", domain.NewValidationError("%s is required", fieldName)
	}
	if !common.IsHexAddress(trimmed) {
		return "", domain.NewValidationError("%s must be a valid Ethereum address", fieldName)
	}

	return common.HexToAddress(trimmed).Hex(), nil
}

func (uc *NettingUsecase) findNettingAgentByWallet(ctx context.Context, fieldName string, walletAddress string) (*domain.Agent, error) {
	agent, err := uc.agentRepository.FindByWalletAddress(ctx, walletAddress)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.NewValidationError("%s references an unknown agent wallet", fieldName)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: find agent for %s: %w", domain.ErrPersistence, fieldName, err)
	}

	return agent, nil
}

type nettingSummary struct {
	Manifest   domain.NettingBatchManifest
	Settlement domain.NettingBatchSettlement
}

type nettingPosition struct {
	AgentID uuid.UUID
	Wallet  string
	Net     big.Int
}

func computeNettingSummary(batchID uuid.UUID, intents []domain.PaymentIntent, createdAt time.Time) (*nettingSummary, error) {
	if len(intents) == 0 {
		return nil, domain.NewValidationError("netting batch must include at least one payment intent")
	}

	positions := map[uuid.UUID]*nettingPosition{}
	grossAmount := big.NewInt(0)
	manifestIntents := make([]domain.NettingManifestIntent, 0, len(intents))
	for _, intent := range intents {
		if !strings.EqualFold(intent.Asset, nettingAssetRUSD) {
			return nil, fmt.Errorf("%w: payment intent %s uses unsupported asset %s", domain.ErrUnsupportedAsset, intent.ID, intent.Asset)
		}

		grossAmount.Add(grossAmount, &intent.Amount)
		payer := getNettingPosition(positions, intent.PayerID, intent.PayerWallet)
		payer.Net.Sub(&payer.Net, &intent.Amount)
		payee := getNettingPosition(positions, intent.PayeeID, intent.PayeeWallet)
		payee.Net.Add(&payee.Net, &intent.Amount)

		manifestIntents = append(manifestIntents, domain.NettingManifestIntent{
			ID:     intent.ID,
			Payer:  intent.PayerWallet,
			Payee:  intent.PayeeWallet,
			Amount: intent.Amount.String(),
		})
	}

	debtors := []domain.NettingPartyAmount{}
	creditors := []domain.NettingPartyAmount{}
	for _, position := range positions {
		if position.Net.Sign() < 0 {
			amount := *new(big.Int).Abs(&position.Net)
			debtors = append(debtors, domain.NettingPartyAmount{
				AgentID: position.AgentID,
				Wallet:  position.Wallet,
				Amount:  amount,
			})
		}
		if position.Net.Sign() > 0 {
			amount := *new(big.Int).Set(&position.Net)
			creditors = append(creditors, domain.NettingPartyAmount{
				AgentID: position.AgentID,
				Wallet:  position.Wallet,
				Amount:  amount,
			})
		}
	}

	sortPartyAmounts(debtors)
	sortPartyAmounts(creditors)
	sort.Slice(manifestIntents, func(i, j int) bool {
		return manifestIntents[i].ID.String() < manifestIntents[j].ID.String()
	})

	netAmount := big.NewInt(0)
	for _, debtor := range debtors {
		netAmount.Add(netAmount, &debtor.Amount)
	}

	manifest := domain.NettingBatchManifest{
		Version:          nettingManifestVersion,
		BatchID:          batchID,
		CreatedAt:        createdAt.Format(time.RFC3339),
		Asset:            nettingAssetRUSD,
		GrossIntentCount: len(intents),
		GrossAmount:      grossAmount.String(),
		NetAmount:        netAmount.String(),
		Debtors:          manifestPositions(debtors),
		Creditors:        manifestPositions(creditors),
		Intents:          manifestIntents,
	}

	return &nettingSummary{
		Manifest: manifest,
		Settlement: domain.NettingBatchSettlement{
			BatchID:                 batchID,
			GrossIntentCount:        len(intents),
			SettlementTransferCount: len(debtors) + len(creditors),
			GrossAmount:             *grossAmount,
			NetAmount:               *netAmount,
			Debtors:                 debtors,
			Creditors:               creditors,
			SettledAt:               createdAt,
		},
	}, nil
}

func getNettingPosition(positions map[uuid.UUID]*nettingPosition, agentID uuid.UUID, wallet string) *nettingPosition {
	position, ok := positions[agentID]
	if ok {
		return position
	}

	position = &nettingPosition{AgentID: agentID, Wallet: wallet}
	positions[agentID] = position
	return position
}

func sortPartyAmounts(parties []domain.NettingPartyAmount) {
	sort.Slice(parties, func(i, j int) bool {
		return strings.ToLower(parties[i].Wallet) < strings.ToLower(parties[j].Wallet)
	})
}

func manifestPositions(parties []domain.NettingPartyAmount) []domain.NettingManifestPosition {
	positions := make([]domain.NettingManifestPosition, 0, len(parties))
	for _, party := range parties {
		positions = append(positions, domain.NettingManifestPosition{
			Agent:  party.Wallet,
			Amount: party.Amount.String(),
		})
	}

	return positions
}

func paymentIntentResponse(intent *domain.PaymentIntent) *domain.PaymentIntentResponse {
	response := &domain.PaymentIntentResponse{
		ID:             intent.ID,
		IdempotencyKey: intent.IdempotencyKey,
		Payer:          intent.PayerWallet,
		Payee:          intent.PayeeWallet,
		Amount:         intent.Amount.String(),
		Asset:          intent.Asset,
		Status:         string(intent.Status),
		NettingBatchID: intent.NettingBatchID,
		CreatedAt:      intent.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      intent.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if intent.SettledAt != nil {
		settledAt := intent.SettledAt.UTC().Format(time.RFC3339)
		response.SettledAt = &settledAt
	}

	return response
}
