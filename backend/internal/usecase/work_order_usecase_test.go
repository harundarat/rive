package usecase

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

const (
	testRootHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testTxHash   = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var (
	fixedWorkOrderID = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a1f")
	fixedPayerID     = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a20")
	fixedPayeeID     = uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a21")
	fixedTime        = time.Date(2026, 4, 22, 10, 30, 0, 0, time.UTC)
)

type fakeZGStorage struct {
	data  any
	err   error
	calls int
}

func (s *fakeZGStorage) UploadJSON(ctx context.Context, data any) (*domain.ZGUploadOutput, error) {
	s.calls++
	s.data = data
	if s.err != nil {
		return nil, s.err
	}

	return &domain.ZGUploadOutput{RootHash: testRootHash, TxHash: testTxHash}, nil
}

func (s *fakeZGStorage) UploadBytes(ctx context.Context, data []byte) (*domain.ZGUploadOutput, error) {
	s.calls++
	s.data = data
	if s.err != nil {
		return nil, s.err
	}

	return &domain.ZGUploadOutput{RootHash: testRootHash, TxHash: testTxHash}, nil
}

type fakeWorkOrderRepository struct {
	byIdempotencyKey       map[string]domain.WorkOrder
	byOnchainOrderID       map[string]domain.WorkOrder
	findErr                error
	findOnchainErr         error
	findOnchainCalls       int
	createErr              error
	recordErr              error
	rollbackErr            error
	releaseErr             error
	releaseRollbackErr     error
	refundErr              error
	refundRollbackErr      error
	deliveryFindErr        error
	submitDeliveryErr      error
	created                *domain.WorkOrder
	deliveryTarget         *domain.WorkOrderDeliveryTarget
	recordInput            *domain.OrderCreatedWorkOrderUpdate
	rollbackInput          *domain.OrderCreatedWorkOrderRollback
	releaseInput           *domain.OrderReleasedWorkOrderUpdate
	releaseRollbackInput   *domain.OrderReleasedWorkOrderRollback
	refundInput            *domain.OrderRefundedWorkOrderUpdate
	refundRollbackInput    *domain.OrderRefundedWorkOrderRollback
	submitDeliveryInput    *domain.WorkOrderDeliveryUpdate
	recordUpdated          bool
	rollbackUpdated        bool
	releaseUpdated         bool
	releaseRollbackUpdated bool
	refundUpdated          bool
	refundRollbackUpdated  bool
	submitDeliveryUpdated  bool
	findCalls              int
	createCalls            int
	deliveryFindCalls      int
	submitDeliveryCalls    int
	recordCalls            int
	rollbackCalls          int
	releaseCalls           int
	releaseRollbackCalls   int
	refundCalls            int
	refundRollbackCalls    int
}

func (r *fakeWorkOrderRepository) FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.WorkOrder, error) {
	r.findCalls++
	if r.findErr != nil {
		return nil, r.findErr
	}
	if r.byIdempotencyKey != nil {
		if workOrder, ok := r.byIdempotencyKey[idempotencyKey]; ok {
			copy := workOrder
			return &copy, nil
		}
	}

	return nil, domain.ErrNotFound
}

func (r *fakeWorkOrderRepository) FindByOnchainOrderID(ctx context.Context, onchainOrderID big.Int) (*domain.WorkOrder, error) {
	r.findOnchainCalls++
	if r.findOnchainErr != nil {
		return nil, r.findOnchainErr
	}
	if r.byOnchainOrderID != nil {
		if workOrder, ok := r.byOnchainOrderID[onchainOrderID.String()]; ok {
			copy := workOrder
			copy.Amount = *new(big.Int).Set(&workOrder.Amount)
			if workOrder.OnchainOrderID != nil {
				copy.OnchainOrderID = new(big.Int).Set(workOrder.OnchainOrderID)
			}
			return &copy, nil
		}
	}

	return nil, domain.ErrNotFound
}

func (r *fakeWorkOrderRepository) FindDeliveryTargetByOnchainOrderID(ctx context.Context, onchainOrderID big.Int) (*domain.WorkOrderDeliveryTarget, error) {
	r.deliveryFindCalls++
	if r.deliveryFindErr != nil {
		return nil, r.deliveryFindErr
	}
	if r.deliveryTarget == nil {
		return nil, domain.ErrNotFound
	}

	copy := *r.deliveryTarget
	copy.WorkOrder.Amount = *new(big.Int).Set(&r.deliveryTarget.WorkOrder.Amount)
	if r.deliveryTarget.WorkOrder.OnchainOrderID != nil {
		copy.WorkOrder.OnchainOrderID = new(big.Int).Set(r.deliveryTarget.WorkOrder.OnchainOrderID)
	}

	return &copy, nil
}

func (r *fakeWorkOrderRepository) Create(ctx context.Context, workOrder domain.WorkOrder) error {
	r.createCalls++
	copy := workOrder
	r.created = &copy
	if r.createErr != nil {
		return r.createErr
	}
	if r.byIdempotencyKey == nil {
		r.byIdempotencyKey = map[string]domain.WorkOrder{}
	}
	r.byIdempotencyKey[workOrder.IdempotencyKey] = copy

	return nil
}

func (r *fakeWorkOrderRepository) SubmitDelivery(ctx context.Context, update domain.WorkOrderDeliveryUpdate) (bool, error) {
	r.submitDeliveryCalls++
	copy := update
	r.submitDeliveryInput = &copy
	if r.submitDeliveryErr != nil {
		return false, r.submitDeliveryErr
	}

	return r.submitDeliveryUpdated, nil
}

func (r *fakeWorkOrderRepository) RecordOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (bool, error) {
	r.recordCalls++
	copy := event
	r.recordInput = &copy
	if r.recordErr != nil {
		return false, r.recordErr
	}

	return r.recordUpdated, nil
}

func (r *fakeWorkOrderRepository) RollbackOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (bool, error) {
	r.rollbackCalls++
	copy := event
	r.rollbackInput = &copy
	if r.rollbackErr != nil {
		return false, r.rollbackErr
	}

	return r.rollbackUpdated, nil
}

func (r *fakeWorkOrderRepository) RecordOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (bool, error) {
	r.releaseCalls++
	copy := event
	r.releaseInput = &copy
	if r.releaseErr != nil {
		return false, r.releaseErr
	}

	return r.releaseUpdated, nil
}

func (r *fakeWorkOrderRepository) RollbackOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (bool, error) {
	r.releaseRollbackCalls++
	copy := event
	r.releaseRollbackInput = &copy
	if r.releaseRollbackErr != nil {
		return false, r.releaseRollbackErr
	}

	return r.releaseRollbackUpdated, nil
}

func (r *fakeWorkOrderRepository) RecordOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (bool, error) {
	r.refundCalls++
	copy := event
	r.refundInput = &copy
	if r.refundErr != nil {
		return false, r.refundErr
	}

	return r.refundUpdated, nil
}

func (r *fakeWorkOrderRepository) RollbackOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (bool, error) {
	r.refundRollbackCalls++
	copy := event
	r.refundRollbackInput = &copy
	if r.refundRollbackErr != nil {
		return false, r.refundRollbackErr
	}

	return r.refundRollbackUpdated, nil
}

type fakeAgentRepository struct {
	byWallet map[string]domain.Agent
	err      error
	calls    []string
}

func (r *fakeAgentRepository) FindByWalletAddress(ctx context.Context, walletAddress string) (*domain.Agent, error) {
	r.calls = append(r.calls, walletAddress)
	if r.err != nil {
		return nil, r.err
	}
	if agent, ok := r.byWallet[walletAddress]; ok {
		copy := agent
		return &copy, nil
	}

	return nil, domain.ErrNotFound
}

func (r *fakeAgentRepository) Create(ctx context.Context, agent domain.Agent) error {
	if r.byWallet == nil {
		r.byWallet = map[string]domain.Agent{}
	}
	r.byWallet[agent.WalletAddress] = agent
	return nil
}

func TestWorkOrderUsecaseUploadSpecStoresSpecAndWorkOrder(t *testing.T) {
	storage := &fakeZGStorage{}
	workOrders := &fakeWorkOrderRepository{}
	uc := newTestWorkOrderUsecase(storage, workOrders, validAgentRepository())

	output, err := uc.UploadSpec(context.Background(), validWorkOrderSpecRequest())
	if err != nil {
		t.Fatalf("UploadSpec returned error: %v", err)
	}

	if output.ID != fixedWorkOrderID {
		t.Fatalf("expected generated id, got %q", output.ID)
	}
	if output.RootHash != testRootHash {
		t.Fatalf("expected root hash from storage, got %q", output.RootHash)
	}
	if output.TxHash != testTxHash {
		t.Fatalf("expected tx hash from storage, got %q", output.TxHash)
	}

	spec, ok := storage.data.(domain.WorkOrderSpec)
	if !ok {
		t.Fatalf("expected storage data to be WorkOrderSpec, got %T", storage.data)
	}
	if spec.Version != "1.0" {
		t.Fatalf("expected version 1.0, got %q", spec.Version)
	}
	if spec.ID != fixedWorkOrderID {
		t.Fatalf("expected generated spec id, got %q", spec.ID)
	}
	if spec.CreatedAt != "2026-04-22T10:30:00Z" {
		t.Fatalf("expected generated createdAt, got %q", spec.CreatedAt)
	}
	if spec.Parties.Payer != "0xabc" || spec.Parties.Payee != "0xdef" {
		t.Fatalf("expected parties to be preserved, got %+v", spec.Parties)
	}

	if workOrders.created == nil {
		t.Fatal("expected work order to be persisted")
	}
	created := workOrders.created
	if created.ID != fixedWorkOrderID {
		t.Fatalf("expected persisted work order id, got %s", created.ID)
	}
	if created.IdempotencyKey != "wo-request-1" {
		t.Fatalf("expected idempotency key to be persisted, got %q", created.IdempotencyKey)
	}
	if created.CreatorID != fixedPayerID || created.ProviderID != fixedPayeeID {
		t.Fatalf("expected creator/provider agent ids, got %s/%s", created.CreatorID, created.ProviderID)
	}
	if created.Amount.String() != "10000000000000000000" {
		t.Fatalf("expected persisted amount, got %s", created.Amount.String())
	}
	if created.Status != domain.WorkOrderStatusDraft {
		t.Fatalf("expected draft status, got %q", created.Status)
	}
	if created.SpecHash != testRootHash || created.SpecVersion != "1.0" || created.SpecTxHash != testTxHash {
		t.Fatalf("expected spec metadata to be persisted, got %+v", created)
	}
	if !created.CreatedAt.Equal(fixedTime) || !created.UpdatedAt.Equal(fixedTime) {
		t.Fatalf("expected fixed timestamps, got %s/%s", created.CreatedAt, created.UpdatedAt)
	}
}

func TestWorkOrderUsecaseUploadSpecReturnsExistingWorkOrderForIdempotencyKey(t *testing.T) {
	existingID := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a99")
	workOrders := &fakeWorkOrderRepository{
		byIdempotencyKey: map[string]domain.WorkOrder{
			"wo-request-1": {
				ID:             existingID,
				IdempotencyKey: "wo-request-1",
				Amount:         bigIntFromString("1000"),
				Status:         domain.WorkOrderStatusDraft,
				SpecHash:       testRootHash,
				SpecVersion:    "1.0",
				SpecTxHash:     testTxHash,
			},
		},
	}
	storage := &fakeZGStorage{}
	uc := newTestWorkOrderUsecase(storage, workOrders, validAgentRepository())
	request := validWorkOrderSpecRequest()
	request.Task.Title = ""

	output, err := uc.UploadSpec(context.Background(), request)
	if err != nil {
		t.Fatalf("UploadSpec returned error: %v", err)
	}

	if output.ID != existingID {
		t.Fatalf("expected existing work order id, got %q", output.ID)
	}
	if output.RootHash != testRootHash || output.TxHash != testTxHash {
		t.Fatalf("expected existing spec metadata, got %+v", output)
	}
	if storage.calls != 0 {
		t.Fatalf("expected no upload for existing idempotency key, got %d calls", storage.calls)
	}
	if workOrders.createCalls != 0 {
		t.Fatalf("expected no create for existing idempotency key, got %d calls", workOrders.createCalls)
	}
}

func TestWorkOrderUsecaseUploadSpecValidation(t *testing.T) {
	tests := []struct {
		name   string
		update func(*domain.WorkOrderSpecRequest)
	}{
		{
			name: "missing idempotency key",
			update: func(request *domain.WorkOrderSpecRequest) {
				request.IdempotencyKey = ""
			},
		},
		{
			name: "missing required field",
			update: func(request *domain.WorkOrderSpecRequest) {
				request.Task.Title = ""
			},
		},
		{
			name: "invalid deadline",
			update: func(request *domain.WorkOrderSpecRequest) {
				request.Deadline = "tomorrow"
			},
		},
		{
			name: "empty acceptance criteria",
			update: func(request *domain.WorkOrderSpecRequest) {
				request.AcceptanceCriteria = nil
			},
		},
		{
			name: "invalid amount",
			update: func(request *domain.WorkOrderSpecRequest) {
				request.Compensation.Amount = "10.5"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := validWorkOrderSpecRequest()
			tt.update(&request)
			uc := newTestWorkOrderUsecase(&fakeZGStorage{}, &fakeWorkOrderRepository{}, validAgentRepository())

			_, err := uc.UploadSpec(context.Background(), request)
			if err == nil {
				t.Fatal("expected validation error")
			}

			var validationErr *domain.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %T", err)
			}
		})
	}
}

func TestWorkOrderUsecaseUploadSpecReturnsValidationErrorForUnknownAgent(t *testing.T) {
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, &fakeWorkOrderRepository{}, &fakeAgentRepository{byWallet: map[string]domain.Agent{}})

	_, err := uc.UploadSpec(context.Background(), validWorkOrderSpecRequest())
	if err == nil {
		t.Fatal("expected validation error")
	}

	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestWorkOrderUsecaseUploadSpecReturnsStorageError(t *testing.T) {
	expectedErr := errors.New("storage unavailable")
	storage := &fakeZGStorage{err: expectedErr}
	workOrders := &fakeWorkOrderRepository{}
	uc := newTestWorkOrderUsecase(storage, workOrders, validAgentRepository())

	_, err := uc.UploadSpec(context.Background(), validWorkOrderSpecRequest())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected storage error, got %v", err)
	}
	if !errors.Is(err, domain.ErrStorage) {
		t.Fatalf("expected domain storage error, got %v", err)
	}
	if workOrders.createCalls != 0 {
		t.Fatalf("expected no persisted work order after storage error, got %d creates", workOrders.createCalls)
	}
}

func TestWorkOrderUsecaseUploadSpecReturnsPersistenceError(t *testing.T) {
	expectedErr := errors.New("database unavailable")
	workOrders := &fakeWorkOrderRepository{createErr: expectedErr}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	_, err := uc.UploadSpec(context.Background(), validWorkOrderSpecRequest())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected persistence error, got %v", err)
	}
	if !errors.Is(err, domain.ErrPersistence) {
		t.Fatalf("expected domain persistence error, got %v", err)
	}
}

func TestWorkOrderUsecaseRecordOrderCreated(t *testing.T) {
	workOrders := &fakeWorkOrderRepository{recordUpdated: true}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())
	recordedAt := time.Date(2026, 4, 22, 17, 30, 0, 0, time.FixedZone("WIB", 7*60*60))

	updated, err := uc.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		SpecHash:        testRootHash,
		Payer:           "0xabc",
		Payee:           "0xdef",
		Amount:          bigIntFromString("1000000"),
		OnchainOrderID:  bigIntFromString("1"),
		TransactionHash: testTxHash,
		RecordedAt:      recordedAt,
	})
	if err != nil {
		t.Fatalf("RecordOrderCreated returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}
	if workOrders.recordCalls != 1 {
		t.Fatalf("expected 1 record call, got %d", workOrders.recordCalls)
	}
	if workOrders.recordInput == nil {
		t.Fatal("expected record input")
	}
	if workOrders.recordInput.SpecHash != testRootHash {
		t.Fatalf("expected spec hash %q, got %q", testRootHash, workOrders.recordInput.SpecHash)
	}
	if workOrders.recordInput.Payer != "0xabc" {
		t.Fatalf("expected payer 0xabc, got %q", workOrders.recordInput.Payer)
	}
	if workOrders.recordInput.Payee != "0xdef" {
		t.Fatalf("expected payee 0xdef, got %q", workOrders.recordInput.Payee)
	}
	if workOrders.recordInput.Amount.String() != "1000000" {
		t.Fatalf("expected amount 1000000, got %s", workOrders.recordInput.Amount.String())
	}
	if workOrders.recordInput.OnchainOrderID.String() != "1" {
		t.Fatalf("expected order id 1, got %s", workOrders.recordInput.OnchainOrderID.String())
	}
	if workOrders.recordInput.TransactionHash != testTxHash {
		t.Fatalf("expected tx hash %q, got %q", testTxHash, workOrders.recordInput.TransactionHash)
	}
	if workOrders.recordInput.RecordedAt.Location() != time.UTC {
		t.Fatalf("expected recorded_at location UTC, got %s", workOrders.recordInput.RecordedAt.Location())
	}
	if !workOrders.recordInput.RecordedAt.Equal(recordedAt.UTC()) {
		t.Fatalf("expected recorded_at %s, got %s", recordedAt.UTC(), workOrders.recordInput.RecordedAt)
	}
}

func TestWorkOrderUsecaseRollbackOrderCreated(t *testing.T) {
	workOrders := &fakeWorkOrderRepository{rollbackUpdated: true}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())
	rolledBackAt := time.Date(2026, 4, 22, 17, 45, 0, 0, time.FixedZone("WIB", 7*60*60))

	updated, err := uc.RollbackOrderCreated(context.Background(), domain.OrderCreatedWorkOrderRollback{
		SpecHash:        testRootHash,
		Payer:           "0xabc",
		Payee:           "0xdef",
		Amount:          bigIntFromString("1000000"),
		OnchainOrderID:  bigIntFromString("1"),
		TransactionHash: testTxHash,
		RolledBackAt:    rolledBackAt,
	})
	if err != nil {
		t.Fatalf("RollbackOrderCreated returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}
	if workOrders.rollbackCalls != 1 {
		t.Fatalf("expected 1 rollback call, got %d", workOrders.rollbackCalls)
	}
	if workOrders.rollbackInput == nil {
		t.Fatal("expected rollback input")
	}
	if workOrders.rollbackInput.SpecHash != testRootHash {
		t.Fatalf("expected spec hash %q, got %q", testRootHash, workOrders.rollbackInput.SpecHash)
	}
	if workOrders.rollbackInput.Payer != "0xabc" {
		t.Fatalf("expected payer 0xabc, got %q", workOrders.rollbackInput.Payer)
	}
	if workOrders.rollbackInput.Payee != "0xdef" {
		t.Fatalf("expected payee 0xdef, got %q", workOrders.rollbackInput.Payee)
	}
	if workOrders.rollbackInput.Amount.String() != "1000000" {
		t.Fatalf("expected amount 1000000, got %s", workOrders.rollbackInput.Amount.String())
	}
	if workOrders.rollbackInput.OnchainOrderID.String() != "1" {
		t.Fatalf("expected order id 1, got %s", workOrders.rollbackInput.OnchainOrderID.String())
	}
	if workOrders.rollbackInput.TransactionHash != testTxHash {
		t.Fatalf("expected tx hash %q, got %q", testTxHash, workOrders.rollbackInput.TransactionHash)
	}
	if workOrders.rollbackInput.RolledBackAt.Location() != time.UTC {
		t.Fatalf("expected rollback time location UTC, got %s", workOrders.rollbackInput.RolledBackAt.Location())
	}
	if !workOrders.rollbackInput.RolledBackAt.Equal(rolledBackAt.UTC()) {
		t.Fatalf("expected rollback time %s, got %s", rolledBackAt.UTC(), workOrders.rollbackInput.RolledBackAt)
	}
}

func TestWorkOrderUsecaseRecordOrderReleased(t *testing.T) {
	workOrders := &fakeWorkOrderRepository{releaseUpdated: true}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())
	recordedAt := time.Date(2026, 4, 22, 17, 50, 0, 0, time.FixedZone("WIB", 7*60*60))

	updated, err := uc.RecordOrderReleased(context.Background(), domain.OrderReleasedWorkOrderUpdate{
		Payee:          "0xdef",
		Amount:         bigIntFromString("1000000"),
		OnchainOrderID: bigIntFromString("1"),
		RecordedAt:     recordedAt,
	})
	if err != nil {
		t.Fatalf("RecordOrderReleased returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}
	if workOrders.releaseCalls != 1 {
		t.Fatalf("expected 1 release call, got %d", workOrders.releaseCalls)
	}
	if workOrders.releaseInput == nil {
		t.Fatal("expected release input")
	}
	if workOrders.releaseInput.Payee != "0xdef" {
		t.Fatalf("expected payee 0xdef, got %q", workOrders.releaseInput.Payee)
	}
	if workOrders.releaseInput.Amount.String() != "1000000" {
		t.Fatalf("expected amount 1000000, got %s", workOrders.releaseInput.Amount.String())
	}
	if workOrders.releaseInput.OnchainOrderID.String() != "1" {
		t.Fatalf("expected order id 1, got %s", workOrders.releaseInput.OnchainOrderID.String())
	}
	if workOrders.releaseInput.RecordedAt.Location() != time.UTC {
		t.Fatalf("expected recorded_at location UTC, got %s", workOrders.releaseInput.RecordedAt.Location())
	}
	if !workOrders.releaseInput.RecordedAt.Equal(recordedAt.UTC()) {
		t.Fatalf("expected recorded_at %s, got %s", recordedAt.UTC(), workOrders.releaseInput.RecordedAt)
	}
}

func TestWorkOrderUsecaseRollbackOrderReleased(t *testing.T) {
	workOrders := &fakeWorkOrderRepository{releaseRollbackUpdated: true}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())
	rolledBackAt := time.Date(2026, 4, 22, 18, 0, 0, 0, time.FixedZone("WIB", 7*60*60))

	updated, err := uc.RollbackOrderReleased(context.Background(), domain.OrderReleasedWorkOrderRollback{
		Payee:          "0xdef",
		Amount:         bigIntFromString("1000000"),
		OnchainOrderID: bigIntFromString("1"),
		RolledBackAt:   rolledBackAt,
	})
	if err != nil {
		t.Fatalf("RollbackOrderReleased returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}
	if workOrders.releaseRollbackCalls != 1 {
		t.Fatalf("expected 1 release rollback call, got %d", workOrders.releaseRollbackCalls)
	}
	if workOrders.releaseRollbackInput == nil {
		t.Fatal("expected release rollback input")
	}
	if workOrders.releaseRollbackInput.RolledBackAt.Location() != time.UTC {
		t.Fatalf("expected rollback time location UTC, got %s", workOrders.releaseRollbackInput.RolledBackAt.Location())
	}
	if !workOrders.releaseRollbackInput.RolledBackAt.Equal(rolledBackAt.UTC()) {
		t.Fatalf("expected rollback time %s, got %s", rolledBackAt.UTC(), workOrders.releaseRollbackInput.RolledBackAt)
	}
}

func TestWorkOrderUsecaseRecordOrderRefunded(t *testing.T) {
	workOrders := &fakeWorkOrderRepository{refundUpdated: true}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())
	recordedAt := time.Date(2026, 4, 22, 18, 10, 0, 0, time.FixedZone("WIB", 7*60*60))

	updated, err := uc.RecordOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderUpdate{
		Payer:          "0xabc",
		Amount:         bigIntFromString("1000000"),
		OnchainOrderID: bigIntFromString("1"),
		RecordedAt:     recordedAt,
	})
	if err != nil {
		t.Fatalf("RecordOrderRefunded returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}
	if workOrders.refundCalls != 1 {
		t.Fatalf("expected 1 refund call, got %d", workOrders.refundCalls)
	}
	if workOrders.refundInput == nil {
		t.Fatal("expected refund input")
	}
	if workOrders.refundInput.Payer != "0xabc" {
		t.Fatalf("expected payer 0xabc, got %q", workOrders.refundInput.Payer)
	}
	if workOrders.refundInput.RecordedAt.Location() != time.UTC {
		t.Fatalf("expected recorded_at location UTC, got %s", workOrders.refundInput.RecordedAt.Location())
	}
	if !workOrders.refundInput.RecordedAt.Equal(recordedAt.UTC()) {
		t.Fatalf("expected recorded_at %s, got %s", recordedAt.UTC(), workOrders.refundInput.RecordedAt)
	}
}

func TestWorkOrderUsecaseRollbackOrderRefunded(t *testing.T) {
	workOrders := &fakeWorkOrderRepository{refundRollbackUpdated: true}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())
	rolledBackAt := time.Date(2026, 4, 22, 18, 20, 0, 0, time.FixedZone("WIB", 7*60*60))

	updated, err := uc.RollbackOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderRollback{
		Payer:          "0xabc",
		Amount:         bigIntFromString("1000000"),
		OnchainOrderID: bigIntFromString("1"),
		RolledBackAt:   rolledBackAt,
	})
	if err != nil {
		t.Fatalf("RollbackOrderRefunded returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}
	if workOrders.refundRollbackCalls != 1 {
		t.Fatalf("expected 1 refund rollback call, got %d", workOrders.refundRollbackCalls)
	}
	if workOrders.refundRollbackInput == nil {
		t.Fatal("expected refund rollback input")
	}
	if workOrders.refundRollbackInput.RolledBackAt.Location() != time.UTC {
		t.Fatalf("expected rollback time location UTC, got %s", workOrders.refundRollbackInput.RolledBackAt.Location())
	}
	if !workOrders.refundRollbackInput.RolledBackAt.Equal(rolledBackAt.UTC()) {
		t.Fatalf("expected rollback time %s, got %s", rolledBackAt.UTC(), workOrders.refundRollbackInput.RolledBackAt)
	}
}

func TestWorkOrderUsecaseRecordOrderCreatedReturnsPersistenceError(t *testing.T) {
	expectedErr := errors.New("database unavailable")
	workOrders := &fakeWorkOrderRepository{recordErr: expectedErr}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	_, err := uc.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		SpecHash:        testRootHash,
		Payer:           "0xabc",
		Payee:           "0xdef",
		Amount:          bigIntFromString("1000000"),
		OnchainOrderID:  bigIntFromString("1"),
		TransactionHash: testTxHash,
		RecordedAt:      fixedTime,
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if !errors.Is(err, domain.ErrPersistence) {
		t.Fatalf("expected domain persistence error, got %v", err)
	}
}

func TestWorkOrderUsecaseRollbackOrderCreatedReturnsPersistenceError(t *testing.T) {
	expectedErr := errors.New("database unavailable")
	workOrders := &fakeWorkOrderRepository{rollbackErr: expectedErr}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	_, err := uc.RollbackOrderCreated(context.Background(), domain.OrderCreatedWorkOrderRollback{
		SpecHash:        testRootHash,
		Payer:           "0xabc",
		Payee:           "0xdef",
		Amount:          bigIntFromString("1000000"),
		OnchainOrderID:  bigIntFromString("1"),
		TransactionHash: testTxHash,
		RolledBackAt:    fixedTime,
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if !errors.Is(err, domain.ErrPersistence) {
		t.Fatalf("expected domain persistence error, got %v", err)
	}
}

func TestWorkOrderUsecaseReleaseAndRefundEventsReturnPersistenceError(t *testing.T) {
	expectedErr := errors.New("database unavailable")
	tests := []struct {
		name       string
		workOrders *fakeWorkOrderRepository
		run        func(*WorkOrderUsecase) (bool, error)
	}{
		{
			name:       "record released",
			workOrders: &fakeWorkOrderRepository{releaseErr: expectedErr},
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RecordOrderReleased(context.Background(), domain.OrderReleasedWorkOrderUpdate{
					Payee:          "0xdef",
					Amount:         bigIntFromString("1000000"),
					OnchainOrderID: bigIntFromString("1"),
					RecordedAt:     fixedTime,
				})
			},
		},
		{
			name:       "rollback released",
			workOrders: &fakeWorkOrderRepository{releaseRollbackErr: expectedErr},
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RollbackOrderReleased(context.Background(), domain.OrderReleasedWorkOrderRollback{
					Payee:          "0xdef",
					Amount:         bigIntFromString("1000000"),
					OnchainOrderID: bigIntFromString("1"),
					RolledBackAt:   fixedTime,
				})
			},
		},
		{
			name:       "record refunded",
			workOrders: &fakeWorkOrderRepository{refundErr: expectedErr},
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RecordOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderUpdate{
					Payer:          "0xabc",
					Amount:         bigIntFromString("1000000"),
					OnchainOrderID: bigIntFromString("1"),
					RecordedAt:     fixedTime,
				})
			},
		},
		{
			name:       "rollback refunded",
			workOrders: &fakeWorkOrderRepository{refundRollbackErr: expectedErr},
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RollbackOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderRollback{
					Payer:          "0xabc",
					Amount:         bigIntFromString("1000000"),
					OnchainOrderID: bigIntFromString("1"),
					RolledBackAt:   fixedTime,
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := newTestWorkOrderUsecase(&fakeZGStorage{}, tt.workOrders, validAgentRepository())

			_, err := tt.run(uc)
			if !errors.Is(err, expectedErr) {
				t.Fatalf("expected original error, got %v", err)
			}
			if !errors.Is(err, domain.ErrPersistence) {
				t.Fatalf("expected domain persistence error, got %v", err)
			}
		})
	}
}

func TestWorkOrderUsecaseSubmitDeliverySuccess(t *testing.T) {
	payeeKey := mustGenerateKey(t)
	payee := crypto.PubkeyToAddress(payeeKey.PublicKey).Hex()
	orderID := "123"
	deliveryHash := testRootHash
	workOrders := &fakeWorkOrderRepository{
		deliveryTarget:        validDeliveryTarget(payee, domain.WorkOrderStatusFunded, nil),
		submitDeliveryUpdated: true,
	}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	output, err := uc.SubmitDelivery(context.Background(), orderID, signedDeliveryRequest(t, payeeKey, orderID, deliveryHash))
	if err != nil {
		t.Fatalf("SubmitDelivery returned error: %v", err)
	}

	if output.OnchainOrderID != orderID {
		t.Fatalf("expected onchain order id %q, got %q", orderID, output.OnchainOrderID)
	}
	if output.DeliverableCID != deliveryHash {
		t.Fatalf("expected deliverable cid %q, got %q", deliveryHash, output.DeliverableCID)
	}
	if output.DeliveredAt != fixedTime.Format(time.RFC3339) {
		t.Fatalf("expected delivered_at %q, got %q", fixedTime.Format(time.RFC3339), output.DeliveredAt)
	}
	if workOrders.submitDeliveryInput == nil {
		t.Fatal("expected repository submit delivery call")
	}
	if workOrders.submitDeliveryInput.OnchainOrderID.String() != orderID {
		t.Fatalf("expected repository order id %q, got %q", orderID, workOrders.submitDeliveryInput.OnchainOrderID.String())
	}
	if workOrders.submitDeliveryInput.DeliveryHash != deliveryHash {
		t.Fatalf("expected repository delivery hash %q, got %q", deliveryHash, workOrders.submitDeliveryInput.DeliveryHash)
	}
	if !workOrders.submitDeliveryInput.DeliveredAt.Equal(fixedTime) {
		t.Fatalf("expected repository delivered_at %s, got %s", fixedTime, workOrders.submitDeliveryInput.DeliveredAt)
	}
}

func TestWorkOrderUsecaseSubmitDeliveryRejectsNonPayeeSignature(t *testing.T) {
	payeeKey := mustGenerateKey(t)
	otherKey := mustGenerateKey(t)
	payee := crypto.PubkeyToAddress(payeeKey.PublicKey).Hex()
	workOrders := &fakeWorkOrderRepository{
		deliveryTarget: validDeliveryTarget(payee, domain.WorkOrderStatusFunded, nil),
	}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	_, err := uc.SubmitDelivery(context.Background(), "123", signedDeliveryRequest(t, otherKey, "123", testRootHash))
	if !errors.Is(err, domain.ErrPayeeMismatch) {
		t.Fatalf("expected payee mismatch error, got %v", err)
	}
	if workOrders.submitDeliveryCalls != 0 {
		t.Fatalf("expected no submit delivery call, got %d", workOrders.submitDeliveryCalls)
	}
}

func TestWorkOrderUsecaseSubmitDeliveryRejectsNonFundedOrder(t *testing.T) {
	payeeKey := mustGenerateKey(t)
	payee := crypto.PubkeyToAddress(payeeKey.PublicKey).Hex()

	for _, status := range []domain.WorkOrderStatus{
		domain.WorkOrderStatusDraft,
		domain.WorkOrderStatusCompleted,
		domain.WorkOrderStatusRefunded,
	} {
		t.Run(string(status), func(t *testing.T) {
			workOrders := &fakeWorkOrderRepository{
				deliveryTarget: validDeliveryTarget(payee, status, nil),
			}
			uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

			_, err := uc.SubmitDelivery(context.Background(), "123", signedDeliveryRequest(t, payeeKey, "123", testRootHash))
			if !errors.Is(err, domain.ErrOrderNotFunded) {
				t.Fatalf("expected order not funded error, got %v", err)
			}
			if workOrders.submitDeliveryCalls != 0 {
				t.Fatalf("expected no submit delivery call, got %d", workOrders.submitDeliveryCalls)
			}
		})
	}
}

func TestWorkOrderUsecaseSubmitDeliveryRejectsExistingDelivery(t *testing.T) {
	payeeKey := mustGenerateKey(t)
	payee := crypto.PubkeyToAddress(payeeKey.PublicKey).Hex()
	existingDelivery := testRootHash
	workOrders := &fakeWorkOrderRepository{
		deliveryTarget: validDeliveryTarget(payee, domain.WorkOrderStatusFunded, &existingDelivery),
	}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	_, err := uc.SubmitDelivery(context.Background(), "123", signedDeliveryRequest(t, payeeKey, "123", testRootHash))
	if !errors.Is(err, domain.ErrDeliveryAlreadyPosted) {
		t.Fatalf("expected delivery already posted error, got %v", err)
	}
	if workOrders.submitDeliveryCalls != 0 {
		t.Fatalf("expected no submit delivery call, got %d", workOrders.submitDeliveryCalls)
	}
}

func TestWorkOrderUsecaseSubmitDeliveryClassifiesUpdateRaceAsConflict(t *testing.T) {
	payeeKey := mustGenerateKey(t)
	payee := crypto.PubkeyToAddress(payeeKey.PublicKey).Hex()
	workOrders := &fakeWorkOrderRepository{
		deliveryTarget:        validDeliveryTarget(payee, domain.WorkOrderStatusFunded, nil),
		submitDeliveryUpdated: false,
	}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	_, err := uc.SubmitDelivery(context.Background(), "123", signedDeliveryRequest(t, payeeKey, "123", testRootHash))
	if !errors.Is(err, domain.ErrDeliveryAlreadyPosted) {
		t.Fatalf("expected delivery already posted error, got %v", err)
	}
	if workOrders.deliveryFindCalls != 2 {
		t.Fatalf("expected delivery target to be fetched twice, got %d", workOrders.deliveryFindCalls)
	}
}

func TestWorkOrderUsecaseGetByOnchainOrderIDSuccess(t *testing.T) {
	orderID := bigIntFromString("123")
	deliverable := testRootHash
	deliveredAt := fixedTime
	stored := domain.WorkOrder{
		ID:             fixedWorkOrderID,
		IdempotencyKey: "wo-request-1",
		CreatorID:      fixedPayerID,
		ProviderID:     fixedPayeeID,
		Amount:         bigIntFromString("1000000"),
		Status:         domain.WorkOrderStatusFunded,
		SpecHash:       testRootHash,
		SpecVersion:    "1.0",
		SpecTxHash:     testTxHash,
		DeliverableCID: &deliverable,
		DeliveredAt:    &deliveredAt,
		OnchainOrderID: &orderID,
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
	workOrders := &fakeWorkOrderRepository{
		byOnchainOrderID: map[string]domain.WorkOrder{"123": stored},
	}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	output, err := uc.GetByOnchainOrderID(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetByOnchainOrderID returned error: %v", err)
	}
	if output.ID != fixedWorkOrderID {
		t.Fatalf("expected work order id %s, got %s", fixedWorkOrderID, output.ID)
	}
	if output.Status != domain.WorkOrderStatusFunded {
		t.Fatalf("expected status funded, got %q", output.Status)
	}
	if output.DeliverableCID == nil || *output.DeliverableCID != testRootHash {
		t.Fatalf("expected deliverable cid %q, got %v", testRootHash, output.DeliverableCID)
	}
	if workOrders.findOnchainCalls != 1 {
		t.Fatalf("expected 1 find call, got %d", workOrders.findOnchainCalls)
	}
}

func TestWorkOrderUsecaseGetByOnchainOrderIDAllowsZero(t *testing.T) {
	orderID := bigIntFromString("0")
	stored := domain.WorkOrder{
		ID:             fixedWorkOrderID,
		IdempotencyKey: "wo-request-zero",
		CreatorID:      fixedPayerID,
		ProviderID:     fixedPayeeID,
		Amount:         bigIntFromString("1000000"),
		Status:         domain.WorkOrderStatusFunded,
		SpecHash:       testRootHash,
		SpecVersion:    "1.0",
		SpecTxHash:     testTxHash,
		OnchainOrderID: &orderID,
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
	workOrders := &fakeWorkOrderRepository{
		byOnchainOrderID: map[string]domain.WorkOrder{"0": stored},
	}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	output, err := uc.GetByOnchainOrderID(context.Background(), "0")
	if err != nil {
		t.Fatalf("GetByOnchainOrderID returned error: %v", err)
	}
	if output.OnchainOrderID == nil || output.OnchainOrderID.Sign() != 0 {
		t.Fatalf("expected onchain order id 0, got %v", output.OnchainOrderID)
	}
	if workOrders.findOnchainCalls != 1 {
		t.Fatalf("expected 1 find call, got %d", workOrders.findOnchainCalls)
	}
}

func TestWorkOrderUsecaseGetByOnchainOrderIDValidation(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "empty", id: ""},
		{name: "non-numeric", id: "abc"},
		{name: "negative", id: "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workOrders := &fakeWorkOrderRepository{}
			uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

			_, err := uc.GetByOnchainOrderID(context.Background(), tt.id)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var validationErr *domain.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %T", err)
			}
			if workOrders.findOnchainCalls != 0 {
				t.Fatalf("expected no repo call on validation failure, got %d", workOrders.findOnchainCalls)
			}
		})
	}
}

func TestWorkOrderUsecaseGetByOnchainOrderIDReturnsNotFound(t *testing.T) {
	workOrders := &fakeWorkOrderRepository{}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	_, err := uc.GetByOnchainOrderID(context.Background(), "999")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestWorkOrderUsecaseGetByOnchainOrderIDReturnsPersistenceError(t *testing.T) {
	expectedErr := errors.New("database unavailable")
	workOrders := &fakeWorkOrderRepository{findOnchainErr: expectedErr}
	uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

	_, err := uc.GetByOnchainOrderID(context.Background(), "123")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if !errors.Is(err, domain.ErrPersistence) {
		t.Fatalf("expected domain persistence error, got %v", err)
	}
}

func newTestWorkOrderUsecase(
	storage *fakeZGStorage,
	workOrders *fakeWorkOrderRepository,
	agents *fakeAgentRepository,
) *WorkOrderUsecase {
	uc := NewWorkOrderUsecase(storage, workOrders, agents)
	uc.now = func() time.Time { return fixedTime }
	uc.newID = func() (uuid.UUID, error) { return fixedWorkOrderID, nil }

	return uc
}

func validAgentRepository() *fakeAgentRepository {
	return &fakeAgentRepository{
		byWallet: map[string]domain.Agent{
			"0xabc": {ID: fixedPayerID, WalletAddress: "0xabc"},
			"0xdef": {ID: fixedPayeeID, WalletAddress: "0xdef"},
		},
	}
}

func validWorkOrderSpecRequest() domain.WorkOrderSpecRequest {
	return domain.WorkOrderSpecRequest{
		IdempotencyKey: "wo-request-1",
		Parties: domain.WorkOrderSpecParties{
			Payer: "0xabc",
			Payee: "0xdef",
		},
		Task: domain.WorkOrderSpecTask{
			Title:       "Scrape and clean Yelp reviews for restaurant XYZ",
			Description: "Detailed prose deskripsi tugas...",
			Category:    "data-extraction",
		},
		Deliverable: domain.WorkOrderSpecDeliverable{
			Format: "json",
			Submission: domain.WorkOrderSpecSubmission{
				Method:   "http-callback",
				Endpoint: "https://payee.example/deliver",
			},
		},
		AcceptanceCriteria: []domain.WorkOrderSpecAcceptanceCriteria{
			{
				ID:               "ac1",
				Description:      "Output is valid JSON with >= 100 review objects",
				VerificationHint: nil,
			},
		},
		Compensation: domain.WorkOrderSpecCompensation{
			Amount: "10000000000000000000",
			Asset:  "0G",
			Chain:  "0g-mainnet",
		},
		Deadline: "2026-04-23T10:30:00Z",
	}
}

func bigIntFromString(value string) big.Int {
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic("invalid test big.Int")
	}

	return *amount
}

func validDeliveryTarget(payee string, status domain.WorkOrderStatus, deliverableCID *string) *domain.WorkOrderDeliveryTarget {
	orderID := bigIntFromString("123")

	return &domain.WorkOrderDeliveryTarget{
		Payee: payee,
		WorkOrder: domain.WorkOrder{
			ID:             fixedWorkOrderID,
			IdempotencyKey: "wo-request-1",
			CreatorID:      fixedPayerID,
			ProviderID:     fixedPayeeID,
			Amount:         bigIntFromString("1000000"),
			Status:         status,
			SpecHash:       testRootHash,
			SpecVersion:    "1.0",
			SpecTxHash:     testTxHash,
			DeliverableCID: deliverableCID,
			OnchainOrderID: &orderID,
			CreatedAt:      fixedTime,
			UpdatedAt:      fixedTime,
		},
	}
}

func mustGenerateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	return key
}

func signedDeliveryRequest(t *testing.T, key *ecdsa.PrivateKey, orderID string, deliveryHash string) domain.WorkOrderDeliveryRequest {
	t.Helper()

	message := "deliver:" + orderID + ":" + deliveryHash
	signature, err := crypto.Sign(accounts.TextHash([]byte(message)), key)
	if err != nil {
		t.Fatalf("failed to sign delivery message: %v", err)
	}

	return domain.WorkOrderDeliveryRequest{
		DeliveryHash: deliveryHash,
		Signature:    "0x" + hex.EncodeToString(signature),
	}
}
