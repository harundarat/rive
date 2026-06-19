package usecase

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

var (
	nettingAgentAID = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a30")
	nettingAgentBID = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a31")
	nettingAgentCID = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a32")

	nettingWalletA = "0x1111111111111111111111111111111111111111"
	nettingWalletB = "0x2222222222222222222222222222222222222222"
	nettingWalletC = "0x3333333333333333333333333333333333333333"
)

type fakeNettingRepository struct {
	byKey           map[string]domain.PaymentIntent
	createErr       error
	claim           *domain.NettingBatchClaim
	claimErr        error
	settled         *domain.NettingBatchSettlement
	settleErr       error
	failedBatchID   uuid.UUID
	failedBatchIDs  []uuid.UUID
	failureReason   string
	stuckBatches    []uuid.UUID
	stuckErr        error
	createdIntent   *domain.PaymentIntent
	findCalls       int
	createCalls     int
	claimCalls      int
	markSettleCalls int
	markFailedCalls int
	stuckCalls      int
}

func (r *fakeNettingRepository) FindIntentByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.PaymentIntent, error) {
	r.findCalls++
	if r.byKey != nil {
		if intent, ok := r.byKey[idempotencyKey]; ok {
			copy := copyPaymentIntent(intent)
			return &copy, nil
		}
	}

	return nil, domain.ErrNotFound
}

func (r *fakeNettingRepository) CreateIntentWithAccrual(ctx context.Context, intent domain.PaymentIntent) (*domain.PaymentIntent, error) {
	r.createCalls++
	copy := copyPaymentIntent(intent)
	r.createdIntent = &copy
	if r.createErr != nil {
		return nil, r.createErr
	}
	copy.PayerWallet = nettingWalletA
	copy.PayeeWallet = nettingWalletB

	return &copy, nil
}

func (r *fakeNettingRepository) ClaimPendingIntents(ctx context.Context, windowEnd time.Time, batchID uuid.UUID, claimedAt time.Time) (*domain.NettingBatchClaim, error) {
	r.claimCalls++
	if r.claimErr != nil {
		return nil, r.claimErr
	}
	if r.claim == nil {
		return &domain.NettingBatchClaim{}, nil
	}

	copy := *r.claim
	copy.Batch.ID = batchID
	return &copy, nil
}

func (r *fakeNettingRepository) MarkBatchSettled(ctx context.Context, settlement domain.NettingBatchSettlement) error {
	r.markSettleCalls++
	copy := settlement
	r.settled = &copy
	if r.settleErr != nil {
		return r.settleErr
	}

	return nil
}

func (r *fakeNettingRepository) MarkBatchFailed(ctx context.Context, batchID uuid.UUID, reason string, failedAt time.Time) error {
	r.markFailedCalls++
	r.failedBatchID = batchID
	r.failedBatchIDs = append(r.failedBatchIDs, batchID)
	r.failureReason = reason
	return nil
}

func (r *fakeNettingRepository) FindStuckProcessingBatches(ctx context.Context, olderThan time.Time) ([]uuid.UUID, error) {
	r.stuckCalls++
	if r.stuckErr != nil {
		return nil, r.stuckErr
	}

	return r.stuckBatches, nil
}

type fakeNettingGateway struct {
	instruction   *domain.NettingSettlementInstruction
	txHash        *string
	err           error
	blockUntilCtx bool
	calls         int
}

func (g *fakeNettingGateway) SettleBatch(ctx context.Context, instruction domain.NettingSettlementInstruction) (*domain.NettingSettlementReceipt, error) {
	g.calls++
	copy := instruction
	g.instruction = &copy
	if g.blockUntilCtx {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if g.err != nil {
		return nil, g.err
	}

	return &domain.NettingSettlementReceipt{TransactionHash: g.txHash}, nil
}

func TestNettingUsecaseSubmitIntentCreatesAccrualIntent(t *testing.T) {
	repo := &fakeNettingRepository{}
	uc := newTestNettingUsecase(repo, &fakeNettingGateway{})

	output, err := uc.SubmitIntent(context.Background(), validPaymentIntentRequest())
	if err != nil {
		t.Fatalf("SubmitIntent returned error: %v", err)
	}

	if output.Status != string(domain.PaymentIntentStatusPending) {
		t.Fatalf("expected pending status, got %q", output.Status)
	}
	if repo.createCalls != 1 || repo.createdIntent == nil {
		t.Fatalf("expected payment intent creation")
	}
	if repo.createdIntent.Asset != "rUSD" {
		t.Fatalf("expected asset rUSD, got %q", repo.createdIntent.Asset)
	}
	if repo.createdIntent.Amount.String() != "1000000" {
		t.Fatalf("expected amount 1000000, got %s", repo.createdIntent.Amount.String())
	}
}

func TestNettingUsecaseSubmitIntentReturnsExistingForIdempotencyKey(t *testing.T) {
	existing := validPaymentIntent()
	repo := &fakeNettingRepository{byKey: map[string]domain.PaymentIntent{"intent-1": existing}}
	uc := newTestNettingUsecase(repo, &fakeNettingGateway{})

	output, err := uc.SubmitIntent(context.Background(), validPaymentIntentRequest())
	if err != nil {
		t.Fatalf("SubmitIntent returned error: %v", err)
	}

	if output.ID != existing.ID {
		t.Fatalf("expected existing intent id %s, got %s", existing.ID, output.ID)
	}
	if repo.createCalls != 0 {
		t.Fatalf("expected no create call for idempotent request")
	}
}

func TestNettingUsecaseSubmitIntentRejectsUnsupportedAsset(t *testing.T) {
	repo := &fakeNettingRepository{}
	uc := newTestNettingUsecase(repo, &fakeNettingGateway{})
	request := validPaymentIntentRequest()
	request.Asset = "USDC"

	_, err := uc.SubmitIntent(context.Background(), request)
	if !errors.Is(err, domain.ErrUnsupportedAsset) {
		t.Fatalf("expected unsupported asset error, got %v", err)
	}
}

func TestComputeNettingSummaryMultilateral(t *testing.T) {
	intents := []domain.PaymentIntent{
		testIntent("018f95e4-3f8d-7b70-a4dd-2d9a833c4a41", nettingAgentAID, nettingAgentBID, nettingWalletA, nettingWalletB, 10),
		testIntent("018f95e4-3f8d-7b70-a4dd-2d9a833c4a42", nettingAgentBID, nettingAgentAID, nettingWalletB, nettingWalletA, 4),
		testIntent("018f95e4-3f8d-7b70-a4dd-2d9a833c4a43", nettingAgentAID, nettingAgentCID, nettingWalletA, nettingWalletC, 3),
		testIntent("018f95e4-3f8d-7b70-a4dd-2d9a833c4a44", nettingAgentCID, nettingAgentBID, nettingWalletC, nettingWalletB, 2),
	}

	summary, err := computeNettingSummary(uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4aff"), intents, fixedTime)
	if err != nil {
		t.Fatalf("computeNettingSummary returned error: %v", err)
	}

	if summary.Settlement.GrossAmount.String() != "19" {
		t.Fatalf("expected gross amount 19, got %s", summary.Settlement.GrossAmount.String())
	}
	if summary.Settlement.NetAmount.String() != "9" {
		t.Fatalf("expected net amount 9, got %s", summary.Settlement.NetAmount.String())
	}
	if len(summary.Settlement.Debtors) != 1 || summary.Settlement.Debtors[0].Wallet != nettingWalletA || summary.Settlement.Debtors[0].Amount.String() != "9" {
		t.Fatalf("unexpected debtors: %+v", summary.Settlement.Debtors)
	}
	if len(summary.Settlement.Creditors) != 2 {
		t.Fatalf("expected two creditors, got %+v", summary.Settlement.Creditors)
	}
}

func TestNettingUsecaseFlushPendingUploadsAndSettles(t *testing.T) {
	txHash := "0xsettlement"
	repo := &fakeNettingRepository{
		claim: &domain.NettingBatchClaim{
			Batch: domain.NettingBatch{ID: uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4aff")},
			Intents: []domain.PaymentIntent{
				testIntent("018f95e4-3f8d-7b70-a4dd-2d9a833c4a41", nettingAgentAID, nettingAgentBID, nettingWalletA, nettingWalletB, 10),
				testIntent("018f95e4-3f8d-7b70-a4dd-2d9a833c4a42", nettingAgentBID, nettingAgentAID, nettingWalletB, nettingWalletA, 4),
			},
		},
	}
	gateway := &fakeNettingGateway{txHash: &txHash}
	storage := &fakeZGStorage{}
	uc := newTestNettingUsecaseWithStorage(storage, repo, gateway)

	settlement, err := uc.FlushPending(context.Background())
	if err != nil {
		t.Fatalf("FlushPending returned error: %v", err)
	}

	if storage.calls != 1 {
		t.Fatalf("expected manifest upload")
	}
	if gateway.calls != 1 || gateway.instruction == nil {
		t.Fatalf("expected settlement gateway call")
	}
	if gateway.instruction.SkipOnchainTx {
		t.Fatalf("expected on-chain tx for non-zero net settlement")
	}
	if repo.markSettleCalls != 1 || repo.settled == nil {
		t.Fatalf("expected batch settled")
	}
	if settlement.SettlementTxHash == nil || *settlement.SettlementTxHash != txHash {
		t.Fatalf("expected settlement tx hash %q, got %v", txHash, settlement.SettlementTxHash)
	}
	if settlement.BatchHash != testRootHash {
		t.Fatalf("expected batch hash %q, got %q", testRootHash, settlement.BatchHash)
	}
	if repo.settled.BatchHash != testRootHash {
		t.Fatalf("expected persisted batch hash %q, got %q", testRootHash, repo.settled.BatchHash)
	}
}

func TestNettingUsecaseReconcileStuckBatchesMarksFailed(t *testing.T) {
	stuckA := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4b01")
	stuckB := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4b02")
	repo := &fakeNettingRepository{stuckBatches: []uuid.UUID{stuckA, stuckB}}
	uc := newTestNettingUsecase(repo, &fakeNettingGateway{})

	if err := uc.ReconcileStuckBatches(context.Background()); err != nil {
		t.Fatalf("ReconcileStuckBatches returned error: %v", err)
	}

	if repo.stuckCalls != 1 {
		t.Fatalf("expected one lookup for stuck batches, got %d", repo.stuckCalls)
	}
	if repo.markFailedCalls != 2 {
		t.Fatalf("expected two batches marked failed, got %d", repo.markFailedCalls)
	}
	if len(repo.failedBatchIDs) != 2 || repo.failedBatchIDs[0] != stuckA || repo.failedBatchIDs[1] != stuckB {
		t.Fatalf("unexpected failed batch ids: %+v", repo.failedBatchIDs)
	}
	if repo.failureReason != reconcileStuckReason {
		t.Fatalf("expected reconcile reason %q, got %q", reconcileStuckReason, repo.failureReason)
	}
}

func TestNettingUsecaseReconcileStuckBatchesNoopWhenNone(t *testing.T) {
	repo := &fakeNettingRepository{}
	uc := newTestNettingUsecase(repo, &fakeNettingGateway{})

	if err := uc.ReconcileStuckBatches(context.Background()); err != nil {
		t.Fatalf("ReconcileStuckBatches returned error: %v", err)
	}

	if repo.stuckCalls != 1 {
		t.Fatalf("expected one lookup for stuck batches, got %d", repo.stuckCalls)
	}
	if repo.markFailedCalls != 0 {
		t.Fatalf("expected no batches marked failed, got %d", repo.markFailedCalls)
	}
}

func TestNettingUsecaseFlushPendingSettleTimeoutMarksFailed(t *testing.T) {
	repo := &fakeNettingRepository{
		claim: &domain.NettingBatchClaim{
			Batch: domain.NettingBatch{ID: uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4aff")},
			Intents: []domain.PaymentIntent{
				testIntent("018f95e4-3f8d-7b70-a4dd-2d9a833c4a41", nettingAgentAID, nettingAgentBID, nettingWalletA, nettingWalletB, 10),
				testIntent("018f95e4-3f8d-7b70-a4dd-2d9a833c4a42", nettingAgentBID, nettingAgentAID, nettingWalletB, nettingWalletA, 4),
			},
		},
	}
	gateway := &fakeNettingGateway{blockUntilCtx: true}
	uc := newTestNettingUsecaseWithStorage(&fakeZGStorage{}, repo, gateway)
	uc.settleTimeout = 10 * time.Millisecond

	// ClaimPendingIntents stamps the batch with the generated id (newID stub).
	generatedBatchID := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a40")

	_, err := uc.FlushPending(context.Background())
	if !errors.Is(err, domain.ErrSettlement) {
		t.Fatalf("expected settlement error, got %v", err)
	}
	if gateway.calls != 1 {
		t.Fatalf("expected settlement gateway call, got %d", gateway.calls)
	}
	if repo.markFailedCalls != 1 || repo.failedBatchID != generatedBatchID {
		t.Fatalf("expected stuck batch marked failed, calls=%d id=%v", repo.markFailedCalls, repo.failedBatchID)
	}
	if repo.markSettleCalls != 0 {
		t.Fatalf("expected no settled mark on timeout, got %d", repo.markSettleCalls)
	}
}

func newTestNettingUsecase(repo *fakeNettingRepository, gateway *fakeNettingGateway) *NettingUsecase {
	return newTestNettingUsecaseWithStorage(&fakeZGStorage{}, repo, gateway)
}

func newTestNettingUsecaseWithStorage(storage *fakeZGStorage, repo *fakeNettingRepository, gateway *fakeNettingGateway) *NettingUsecase {
	uc := NewNettingUsecase(storage, repo, validNettingAgentRepository(), gateway, time.Minute)
	uc.now = func() time.Time { return fixedTime }
	uc.newID = func() (uuid.UUID, error) {
		return uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a40"), nil
	}

	return uc
}

func validNettingAgentRepository() *fakeAgentRepository {
	return &fakeAgentRepository{byWallet: map[string]domain.Agent{
		nettingWalletA: {ID: nettingAgentAID, WalletAddress: nettingWalletA},
		nettingWalletB: {ID: nettingAgentBID, WalletAddress: nettingWalletB},
		nettingWalletC: {ID: nettingAgentCID, WalletAddress: nettingWalletC},
	}}
}

func validPaymentIntentRequest() domain.PaymentIntentRequest {
	return domain.PaymentIntentRequest{
		IdempotencyKey: "intent-1",
		Payer:          nettingWalletA,
		Payee:          nettingWalletB,
		Amount:         "1000000",
		Asset:          "rUSD",
	}
}

func validPaymentIntent() domain.PaymentIntent {
	amount := big.NewInt(1000000)
	return domain.PaymentIntent{
		ID:             uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a41"),
		IdempotencyKey: "intent-1",
		PayerID:        nettingAgentAID,
		PayeeID:        nettingAgentBID,
		PayerWallet:    nettingWalletA,
		PayeeWallet:    nettingWalletB,
		Amount:         *amount,
		Asset:          "rUSD",
		Status:         domain.PaymentIntentStatusPending,
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

func testIntent(id string, payerID uuid.UUID, payeeID uuid.UUID, payerWallet string, payeeWallet string, amount int64) domain.PaymentIntent {
	value := big.NewInt(amount)
	return domain.PaymentIntent{
		ID:             uuid.MustParse(id),
		IdempotencyKey: id,
		PayerID:        payerID,
		PayeeID:        payeeID,
		PayerWallet:    payerWallet,
		PayeeWallet:    payeeWallet,
		Amount:         *value,
		Asset:          "rUSD",
		Status:         domain.PaymentIntentStatusPending,
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

func copyPaymentIntent(intent domain.PaymentIntent) domain.PaymentIntent {
	copy := intent
	copy.Amount = *new(big.Int).Set(&intent.Amount)
	return copy
}
