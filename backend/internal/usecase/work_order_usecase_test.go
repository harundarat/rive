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

func (s *fakeZGStorage) UploadJSON(ctx context.Context, data any) (*domain.StorageUploadOutput, error) {
	s.calls++
	s.data = data
	if s.err != nil {
		return nil, s.err
	}

	return &domain.StorageUploadOutput{RootHash: testRootHash, TxHash: testTxHash}, nil
}

func (s *fakeZGStorage) UploadBytes(ctx context.Context, data []byte) (*domain.StorageUploadOutput, error) {
	s.calls++
	s.data = data
	if s.err != nil {
		return nil, s.err
	}

	return &domain.StorageUploadOutput{RootHash: testRootHash, TxHash: testTxHash}, nil
}

type fakeWorkOrderRepository struct {
	byIdempotencyKey      map[string]domain.WorkOrder
	byOnchainOrderID      map[string]domain.WorkOrder
	findErr               error
	findOnchainErr        error
	findOnchainCalls      int
	createErr             error
	deliveryFindErr       error
	submitDeliveryErr     error
	created               *domain.WorkOrder
	deliveryTarget        *domain.WorkOrderDeliveryTarget
	submitDeliveryInput   *domain.WorkOrderDeliveryUpdate
	submitDeliveryUpdated bool
	findCalls             int
	createCalls           int
	deliveryFindCalls     int
	submitDeliveryCalls   int

	// Escrow resolve/apply orchestration (Fase 3). All six Resolve*/Apply* pairs
	// delegate to the shared resolve()/apply() helpers, so a single set of fields
	// drives every escrow event in tests.
	resolveTarget  *domain.WorkOrderBookkeepingTarget
	resolveMatched bool
	resolveErr     error
	resolveCalls   int
	applyErr       error
	applyUpdated   bool
	applyCalls     int
	applyEntry     *domain.EscrowJournalEntry
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

func (r *fakeWorkOrderRepository) resolve() (*domain.WorkOrderBookkeepingTarget, bool, error) {
	r.resolveCalls++
	if r.resolveErr != nil {
		return nil, false, r.resolveErr
	}

	return r.resolveTarget, r.resolveMatched, nil
}

func (r *fakeWorkOrderRepository) apply(entry domain.EscrowJournalEntry) (bool, error) {
	r.applyCalls++
	captured := entry
	r.applyEntry = &captured
	if r.applyErr != nil {
		return false, r.applyErr
	}

	return r.applyUpdated, nil
}

func (r *fakeWorkOrderRepository) ResolveOrderCreatedTarget(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolve()
}

func (r *fakeWorkOrderRepository) ApplyOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	return r.apply(entry)
}

func (r *fakeWorkOrderRepository) ResolveOrderCreatedRollbackTarget(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolve()
}

func (r *fakeWorkOrderRepository) ApplyOrderCreatedRollback(ctx context.Context, event domain.OrderCreatedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	return r.apply(entry)
}

func (r *fakeWorkOrderRepository) ResolveOrderReleasedTarget(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolve()
}

func (r *fakeWorkOrderRepository) ApplyOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	return r.apply(entry)
}

func (r *fakeWorkOrderRepository) ResolveOrderReleasedRollbackTarget(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolve()
}

func (r *fakeWorkOrderRepository) ApplyOrderReleasedRollback(ctx context.Context, event domain.OrderReleasedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	return r.apply(entry)
}

func (r *fakeWorkOrderRepository) ResolveOrderRefundedTarget(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolve()
}

func (r *fakeWorkOrderRepository) ApplyOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate, entry domain.EscrowJournalEntry) (bool, error) {
	return r.apply(entry)
}

func (r *fakeWorkOrderRepository) ResolveOrderRefundedRollbackTarget(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (*domain.WorkOrderBookkeepingTarget, bool, error) {
	return r.resolve()
}

func (r *fakeWorkOrderRepository) ApplyOrderRefundedRollback(ctx context.Context, event domain.OrderRefundedWorkOrderRollback, entry domain.EscrowJournalEntry) (bool, error) {
	return r.apply(entry)
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

func TestWorkOrderUsecaseEscrowEventsBuildAndPersistJournal(t *testing.T) {
	onchainOrderID := bigIntFromString("1")
	amount := bigIntFromString("1000000")
	occurredAt := time.Date(2026, 4, 22, 17, 30, 0, 0, time.FixedZone("WIB", 7*60*60))
	occurredUTC := occurredAt.UTC()

	tests := []struct {
		name        string
		run         func(*WorkOrderUsecase) (bool, error)
		action      string
		description string
		postings    []domain.LedgerPosting
	}{
		{
			name: "record order created",
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
					SpecHash: testRootHash, Payer: "0xabc", Payee: "0xdef", Amount: amount, OnchainOrderID: onchainOrderID, TransactionHash: testTxHash, RecordedAt: occurredAt,
				})
			},
			action:      "order_created",
			description: "Escrow order created for on-chain order 1",
			postings: []domain.LedgerPosting{
				{AgentID: fixedPayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: fixedPayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeCredit},
			},
		},
		{
			name: "rollback order created",
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RollbackOrderCreated(context.Background(), domain.OrderCreatedWorkOrderRollback{
					SpecHash: testRootHash, Payer: "0xabc", Payee: "0xdef", Amount: amount, OnchainOrderID: onchainOrderID, TransactionHash: testTxHash, RolledBackAt: occurredAt,
				})
			},
			action:      "order_created_rollback",
			description: "Escrow order created rollback for on-chain order 1",
			postings: []domain.LedgerPosting{
				{AgentID: fixedPayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: fixedPayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeCredit},
			},
		},
		{
			name: "record order released",
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RecordOrderReleased(context.Background(), domain.OrderReleasedWorkOrderUpdate{
					Payee: "0xdef", Amount: amount, OnchainOrderID: onchainOrderID, TransactionHash: testTxHash, RecordedAt: occurredAt,
				})
			},
			action:      "order_released",
			description: "Escrow order released for on-chain order 1",
			postings: []domain.LedgerPosting{
				{AgentID: fixedPayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: fixedPayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeCredit},
				{AgentID: fixedPayerID, AccountName: domain.AccountNameServiceExpense, AccountType: domain.AccountTypeExpense, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: fixedPayeeID, AccountName: domain.AccountNameServiceRevenue, AccountType: domain.AccountTypeRevenue, EntryType: domain.LedgerEntryTypeCredit},
			},
		},
		{
			name: "rollback order released",
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RollbackOrderReleased(context.Background(), domain.OrderReleasedWorkOrderRollback{
					Payee: "0xdef", Amount: amount, OnchainOrderID: onchainOrderID, TransactionHash: testTxHash, RolledBackAt: occurredAt,
				})
			},
			action:      "order_released_rollback",
			description: "Escrow order released rollback for on-chain order 1",
			postings: []domain.LedgerPosting{
				{AgentID: fixedPayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: fixedPayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeCredit},
				{AgentID: fixedPayeeID, AccountName: domain.AccountNameServiceRevenue, AccountType: domain.AccountTypeRevenue, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: fixedPayerID, AccountName: domain.AccountNameServiceExpense, AccountType: domain.AccountTypeExpense, EntryType: domain.LedgerEntryTypeCredit},
			},
		},
		{
			name: "record order refunded",
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RecordOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderUpdate{
					Payer: "0xabc", Amount: amount, OnchainOrderID: onchainOrderID, TransactionHash: testTxHash, RecordedAt: occurredAt,
				})
			},
			action:      "order_refunded",
			description: "Escrow order refunded for on-chain order 1",
			postings: []domain.LedgerPosting{
				{AgentID: fixedPayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: fixedPayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeCredit},
			},
		},
		{
			name: "rollback order refunded",
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RollbackOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderRollback{
					Payer: "0xabc", Amount: amount, OnchainOrderID: onchainOrderID, TransactionHash: testTxHash, RolledBackAt: occurredAt,
				})
			},
			action:      "order_refunded_rollback",
			description: "Escrow order refunded rollback for on-chain order 1",
			postings: []domain.LedgerPosting{
				{AgentID: fixedPayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: fixedPayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeCredit},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &fakeZGStorage{}
			workOrders := &fakeWorkOrderRepository{
				resolveMatched: true,
				resolveTarget:  &domain.WorkOrderBookkeepingTarget{WorkOrderID: fixedWorkOrderID, PayerID: fixedPayerID, PayeeID: fixedPayeeID},
				applyUpdated:   true,
			}
			uc := newTestWorkOrderUsecase(storage, workOrders, validAgentRepository())

			updated, err := tt.run(uc)
			if err != nil {
				t.Fatalf("event returned error: %v", err)
			}
			if !updated {
				t.Fatal("expected updated=true")
			}

			// Ordering: resolve, then exactly one upload, then apply.
			if workOrders.resolveCalls != 1 || storage.calls != 1 || workOrders.applyCalls != 1 {
				t.Fatalf("expected resolve=1 upload=1 apply=1, got resolve=%d upload=%d apply=%d", workOrders.resolveCalls, storage.calls, workOrders.applyCalls)
			}
			if workOrders.applyEntry == nil {
				t.Fatal("expected an entry passed to apply")
			}
			entry := workOrders.applyEntry

			// The upload's root hash must already be on the entry handed to apply,
			// proving the 0G upload happened before (and outside) the DB write.
			if entry.StorageCID != testRootHash {
				t.Fatalf("expected entry storage cid %q, got %q", testRootHash, entry.StorageCID)
			}
			if entry.WorkOrderID != fixedWorkOrderID {
				t.Fatalf("expected work order id %s, got %s", fixedWorkOrderID, entry.WorkOrderID)
			}
			if entry.Description != tt.description {
				t.Fatalf("expected description %q, got %q", tt.description, entry.Description)
			}
			wantKey := escrowJournalKey(fixedWorkOrderID, tt.action, testTxHash, "", "", onchainOrderID)
			if entry.IdempotencyKey != wantKey {
				t.Fatalf("expected idempotency key %q, got %q", wantKey, entry.IdempotencyKey)
			}
			if entry.CreatedAt.Location() != time.UTC || !entry.CreatedAt.Equal(occurredUTC) {
				t.Fatalf("expected created_at %s (UTC), got %s", occurredUTC, entry.CreatedAt)
			}
			if entry.Amount.String() != amount.String() {
				t.Fatalf("expected amount %s, got %s", amount.String(), entry.Amount.String())
			}
			if len(entry.Postings) != len(tt.postings) {
				t.Fatalf("expected %d postings, got %d", len(tt.postings), len(entry.Postings))
			}
			for i, want := range tt.postings {
				if entry.Postings[i] != want {
					t.Fatalf("posting %d: expected %+v, got %+v", i, want, entry.Postings[i])
				}
			}

			// The 0G anchor payload mirrors the entry and keeps its committed schema.
			anchor, ok := storage.data.(bookkeepingJournalAnchor)
			if !ok {
				t.Fatalf("expected bookkeepingJournalAnchor payload, got %T", storage.data)
			}
			if anchor.SchemaVersion != "1.0" || anchor.Kind != "escrow_journal_entry" {
				t.Fatalf("unexpected anchor metadata: %+v", anchor)
			}
			if anchor.JournalEntryID != entry.JournalID || anchor.IdempotencyKey != entry.IdempotencyKey || anchor.WorkOrderID != fixedWorkOrderID {
				t.Fatalf("unexpected anchor identity: %+v", anchor)
			}
			if anchor.CreatedAt != occurredUTC.Format(time.RFC3339Nano) {
				t.Fatalf("expected anchor created_at %q, got %q", occurredUTC.Format(time.RFC3339Nano), anchor.CreatedAt)
			}
			if len(anchor.Postings) != len(tt.postings) {
				t.Fatalf("expected %d anchor postings, got %d", len(tt.postings), len(anchor.Postings))
			}
			for i, want := range tt.postings {
				got := anchor.Postings[i]
				if got.AccountName != want.AccountName || got.AccountType != want.AccountType || got.EntryType != want.EntryType || got.AgentID != want.AgentID || got.Amount != amount.String() {
					t.Fatalf("anchor posting %d: expected %+v, got %+v", i, want, got)
				}
			}
		})
	}
}

func TestWorkOrderUsecaseEscrowEventNoopSkipsUploadAndApply(t *testing.T) {
	storage := &fakeZGStorage{}
	workOrders := &fakeWorkOrderRepository{resolveMatched: false}
	uc := newTestWorkOrderUsecase(storage, workOrders, validAgentRepository())

	updated, err := uc.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		SpecHash: testRootHash, Payer: "0xabc", Payee: "0xdef", Amount: bigIntFromString("1000000"), OnchainOrderID: bigIntFromString("1"), TransactionHash: testTxHash, RecordedAt: fixedTime,
	})
	if err != nil {
		t.Fatalf("RecordOrderCreated returned error: %v", err)
	}
	if updated {
		t.Fatal("expected updated=false for a non-matching event")
	}
	if storage.calls != 0 {
		t.Fatalf("expected no upload for a no-op event, got %d", storage.calls)
	}
	if workOrders.applyCalls != 0 {
		t.Fatalf("expected no apply for a no-op event, got %d", workOrders.applyCalls)
	}
}

func TestWorkOrderUsecaseEscrowEventUploadFailureSkipsApply(t *testing.T) {
	expectedErr := errors.New("0g unavailable")
	storage := &fakeZGStorage{err: expectedErr}
	workOrders := &fakeWorkOrderRepository{
		resolveMatched: true,
		resolveTarget:  &domain.WorkOrderBookkeepingTarget{WorkOrderID: fixedWorkOrderID, PayerID: fixedPayerID, PayeeID: fixedPayeeID},
		applyUpdated:   true,
	}
	uc := newTestWorkOrderUsecase(storage, workOrders, validAgentRepository())

	_, err := uc.RecordOrderReleased(context.Background(), domain.OrderReleasedWorkOrderUpdate{
		Payee: "0xdef", Amount: bigIntFromString("1000000"), OnchainOrderID: bigIntFromString("1"), RecordedAt: fixedTime,
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected wrapped upload error, got %v", err)
	}
	if !errors.Is(err, domain.ErrStorage) {
		t.Fatalf("expected domain storage error, got %v", err)
	}
	if errors.Is(err, domain.ErrPersistence) {
		t.Fatalf("upload failure must not be classified as persistence, got %v", err)
	}
	if workOrders.applyCalls != 0 {
		t.Fatalf("expected apply to be skipped after upload failure, got %d", workOrders.applyCalls)
	}
}

func TestWorkOrderUsecaseEscrowEventResolveErrorIsPersistence(t *testing.T) {
	expectedErr := errors.New("database unavailable")
	storage := &fakeZGStorage{}
	workOrders := &fakeWorkOrderRepository{resolveErr: expectedErr}
	uc := newTestWorkOrderUsecase(storage, workOrders, validAgentRepository())

	_, err := uc.RecordOrderCreated(context.Background(), domain.OrderCreatedWorkOrderUpdate{
		SpecHash: testRootHash, Payer: "0xabc", Payee: "0xdef", Amount: bigIntFromString("1000000"), OnchainOrderID: bigIntFromString("1"), TransactionHash: testTxHash, RecordedAt: fixedTime,
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if !errors.Is(err, domain.ErrPersistence) {
		t.Fatalf("expected domain persistence error, got %v", err)
	}
	if storage.calls != 0 || workOrders.applyCalls != 0 {
		t.Fatalf("expected no upload/apply after resolve failure, got upload=%d apply=%d", storage.calls, workOrders.applyCalls)
	}
}

func TestWorkOrderUsecaseEscrowEventApplyErrorIsPersistence(t *testing.T) {
	expectedErr := errors.New("database unavailable")
	tests := []struct {
		name string
		run  func(*WorkOrderUsecase) (bool, error)
	}{
		{
			name: "record released",
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RecordOrderReleased(context.Background(), domain.OrderReleasedWorkOrderUpdate{
					Payee: "0xdef", Amount: bigIntFromString("1000000"), OnchainOrderID: bigIntFromString("1"), RecordedAt: fixedTime,
				})
			},
		},
		{
			name: "rollback refunded",
			run: func(uc *WorkOrderUsecase) (bool, error) {
				return uc.RollbackOrderRefunded(context.Background(), domain.OrderRefundedWorkOrderRollback{
					Payer: "0xabc", Amount: bigIntFromString("1000000"), OnchainOrderID: bigIntFromString("1"), RolledBackAt: fixedTime,
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workOrders := &fakeWorkOrderRepository{
				resolveMatched: true,
				resolveTarget:  &domain.WorkOrderBookkeepingTarget{WorkOrderID: fixedWorkOrderID, PayerID: fixedPayerID, PayeeID: fixedPayeeID},
				applyErr:       expectedErr,
			}
			uc := newTestWorkOrderUsecase(&fakeZGStorage{}, workOrders, validAgentRepository())

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
