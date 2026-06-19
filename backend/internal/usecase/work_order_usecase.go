package usecase

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

const workOrderSpecVersion = "1.0"

// Escrow-only double-entry account names. The escrow bookkeeping postings live
// here (not in the repository) because they are domain logic orchestrated by the
// usecase. Names shared with netting (Service Expense/Revenue) come from the
// domain package so both flows post to the same account.
const (
	accountNameEscrowLocked  = "escrow_locked"
	accountNameEscrowPending = "escrow_pending"
)

var hex32Pattern = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

type WorkOrderUsecase struct {
	zgStorage           domain.ZGStorage
	workOrderRepository domain.WorkOrderRepository
	agentRepository     domain.AgentRepository
	now                 func() time.Time
	newID               func() (uuid.UUID, error)
}

func NewWorkOrderUsecase(
	zgStorage domain.ZGStorage,
	workOrderRepository domain.WorkOrderRepository,
	agentRepository domain.AgentRepository,
) *WorkOrderUsecase {
	return &WorkOrderUsecase{
		zgStorage:           zgStorage,
		workOrderRepository: workOrderRepository,
		agentRepository:     agentRepository,
		now:                 time.Now,
		newID:               uuid.NewV7,
	}
}

func (uc *WorkOrderUsecase) UploadSpec(ctx context.Context, request domain.WorkOrderSpecRequest) (*domain.WorkOrderSpecResponse, error) {
	if isBlank(request.IdempotencyKey) {
		return nil, domain.NewValidationError("idempotency_key is required")
	}

	existing, err := uc.workOrderRepository.FindByIdempotencyKey(ctx, request.IdempotencyKey)
	if err == nil {
		return workOrderSpecResponseFromWorkOrder(existing), nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%w: find existing work order: %w", domain.ErrPersistence, err)
	}

	if err := validateWorkOrderSpecRequest(request); err != nil {
		return nil, err
	}

	amount, err := parseWorkOrderAmount(request.Compensation.Amount)
	if err != nil {
		return nil, err
	}

	creator, err := uc.findAgentByWallet(ctx, "parties.payer", request.Parties.Payer)
	if err != nil {
		return nil, err
	}
	provider, err := uc.findAgentByWallet(ctx, "parties.payee", request.Parties.Payee)
	if err != nil {
		return nil, err
	}

	id, err := uc.newID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate work order id: %w", err)
	}
	now := uc.now().UTC()

	spec := domain.WorkOrderSpec{
		Version:            workOrderSpecVersion,
		ID:                 id,
		CreatedAt:          now.Format(time.RFC3339),
		Parties:            request.Parties,
		Task:               request.Task,
		Deliverable:        request.Deliverable,
		AcceptanceCriteria: request.AcceptanceCriteria,
		Compensation:       request.Compensation,
		Deadline:           request.Deadline,
	}

	uploadOutput, err := uc.zgStorage.UploadJSON(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("%w: upload work order spec: %w", domain.ErrStorage, err)
	}

	workOrder := domain.WorkOrder{
		ID:             id,
		IdempotencyKey: request.IdempotencyKey,
		CreatorID:      creator.ID,
		ProviderID:     provider.ID,
		Amount:         amount,
		Status:         domain.WorkOrderStatusDraft,
		SpecHash:       uploadOutput.RootHash,
		SpecVersion:    workOrderSpecVersion,
		SpecTxHash:     uploadOutput.TxHash,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := uc.workOrderRepository.Create(ctx, workOrder); err != nil {
		existing, findErr := uc.workOrderRepository.FindByIdempotencyKey(ctx, request.IdempotencyKey)
		if findErr == nil {
			return workOrderSpecResponseFromWorkOrder(existing), nil
		}

		return nil, fmt.Errorf("%w: create work order: %w", domain.ErrPersistence, err)
	}

	return &domain.WorkOrderSpecResponse{
		ID:       spec.ID,
		RootHash: uploadOutput.RootHash,
		TxHash:   uploadOutput.TxHash,
	}, nil
}

func (uc *WorkOrderUsecase) SubmitDelivery(ctx context.Context, onchainOrderID string, request domain.WorkOrderDeliveryRequest) (*domain.WorkOrderDeliveryResponse, error) {
	orderID, normalizedOrderID, err := parseOnchainOrderID(onchainOrderID)
	if err != nil {
		return nil, err
	}
	if err := validateWorkOrderDeliveryRequest(request); err != nil {
		return nil, err
	}

	target, err := uc.workOrderRepository.FindDeliveryTargetByOnchainOrderID(ctx, orderID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: find delivery target: %w", domain.ErrPersistence, err)
	}

	message := fmt.Sprintf("deliver:%s:%s", normalizedOrderID, request.DeliveryHash)
	recoveredAddress, err := recoverEthereumAddress(message, request.Signature)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidSignature, err)
	}
	if !strings.EqualFold(recoveredAddress, target.Payee) {
		return nil, domain.ErrPayeeMismatch
	}
	if target.WorkOrder.Status != domain.WorkOrderStatusFunded {
		return nil, domain.ErrOrderNotFunded
	}
	if target.WorkOrder.DeliverableCID != nil {
		return nil, domain.ErrDeliveryAlreadyPosted
	}

	deliveredAt := uc.now().UTC()
	updated, err := uc.workOrderRepository.SubmitDelivery(ctx, domain.WorkOrderDeliveryUpdate{
		OnchainOrderID: orderID,
		DeliveryHash:   request.DeliveryHash,
		DeliveredAt:    deliveredAt,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: submit delivery: %w", domain.ErrPersistence, err)
	}
	if !updated {
		return nil, uc.classifyDeliveryUpdateConflict(ctx, orderID)
	}

	return &domain.WorkOrderDeliveryResponse{
		OnchainOrderID: normalizedOrderID,
		DeliverableCID: request.DeliveryHash,
		DeliveredAt:    deliveredAt.Format(time.RFC3339),
	}, nil
}

func (uc *WorkOrderUsecase) GetByOnchainOrderID(ctx context.Context, onchainOrderID string) (*domain.WorkOrder, error) {
	orderID, _, err := parseOnchainOrderID(onchainOrderID)
	if err != nil {
		return nil, err
	}

	workOrder, err := uc.workOrderRepository.FindByOnchainOrderID(ctx, orderID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: find work order: %w", domain.ErrPersistence, err)
	}

	return workOrder, nil
}

func (uc *WorkOrderUsecase) RecordOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (bool, error) {
	event.RecordedAt = event.RecordedAt.UTC()

	return uc.recordEscrowEvent(ctx, escrowEventContext{
		op:             "record order created",
		action:         "order_created",
		description:    fmt.Sprintf("Escrow order created for on-chain order %s", event.OnchainOrderID.String()),
		amount:         event.Amount,
		onchainOrderID: event.OnchainOrderID,
		txHash:         event.TransactionHash,
		blockNumber:    event.BlockNumber,
		logIndex:       event.LogIndex,
		occurredAt:     event.RecordedAt,
		postings: func(target *domain.WorkOrderBookkeepingTarget) []domain.LedgerPosting {
			return []domain.LedgerPosting{
				{AgentID: target.PayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: target.PayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeCredit},
			}
		},
	},
		func(ctx context.Context) (*domain.WorkOrderBookkeepingTarget, bool, error) {
			return uc.workOrderRepository.ResolveOrderCreatedTarget(ctx, event)
		},
		func(ctx context.Context, entry domain.EscrowJournalEntry) (bool, error) {
			return uc.workOrderRepository.ApplyOrderCreated(ctx, event, entry)
		},
	)
}

func (uc *WorkOrderUsecase) RollbackOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (bool, error) {
	event.RolledBackAt = event.RolledBackAt.UTC()

	return uc.recordEscrowEvent(ctx, escrowEventContext{
		op:             "rollback order created",
		action:         "order_created_rollback",
		description:    fmt.Sprintf("Escrow order created rollback for on-chain order %s", event.OnchainOrderID.String()),
		amount:         event.Amount,
		onchainOrderID: event.OnchainOrderID,
		txHash:         event.TransactionHash,
		blockNumber:    event.BlockNumber,
		logIndex:       event.LogIndex,
		occurredAt:     event.RolledBackAt,
		postings: func(target *domain.WorkOrderBookkeepingTarget) []domain.LedgerPosting {
			return []domain.LedgerPosting{
				{AgentID: target.PayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: target.PayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeCredit},
			}
		},
	},
		func(ctx context.Context) (*domain.WorkOrderBookkeepingTarget, bool, error) {
			return uc.workOrderRepository.ResolveOrderCreatedRollbackTarget(ctx, event)
		},
		func(ctx context.Context, entry domain.EscrowJournalEntry) (bool, error) {
			return uc.workOrderRepository.ApplyOrderCreatedRollback(ctx, event, entry)
		},
	)
}

func (uc *WorkOrderUsecase) RecordOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (bool, error) {
	event.RecordedAt = event.RecordedAt.UTC()

	return uc.recordEscrowEvent(ctx, escrowEventContext{
		op:             "record order released",
		action:         "order_released",
		description:    fmt.Sprintf("Escrow order released for on-chain order %s", event.OnchainOrderID.String()),
		amount:         event.Amount,
		onchainOrderID: event.OnchainOrderID,
		txHash:         event.TransactionHash,
		blockNumber:    event.BlockNumber,
		logIndex:       event.LogIndex,
		occurredAt:     event.RecordedAt,
		postings: func(target *domain.WorkOrderBookkeepingTarget) []domain.LedgerPosting {
			return []domain.LedgerPosting{
				{AgentID: target.PayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: target.PayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeCredit},
				{AgentID: target.PayerID, AccountName: domain.AccountNameServiceExpense, AccountType: domain.AccountTypeExpense, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: target.PayeeID, AccountName: domain.AccountNameServiceRevenue, AccountType: domain.AccountTypeRevenue, EntryType: domain.LedgerEntryTypeCredit},
			}
		},
	},
		func(ctx context.Context) (*domain.WorkOrderBookkeepingTarget, bool, error) {
			return uc.workOrderRepository.ResolveOrderReleasedTarget(ctx, event)
		},
		func(ctx context.Context, entry domain.EscrowJournalEntry) (bool, error) {
			return uc.workOrderRepository.ApplyOrderReleased(ctx, event, entry)
		},
	)
}

func (uc *WorkOrderUsecase) RollbackOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (bool, error) {
	event.RolledBackAt = event.RolledBackAt.UTC()

	return uc.recordEscrowEvent(ctx, escrowEventContext{
		op:             "rollback order released",
		action:         "order_released_rollback",
		description:    fmt.Sprintf("Escrow order released rollback for on-chain order %s", event.OnchainOrderID.String()),
		amount:         event.Amount,
		onchainOrderID: event.OnchainOrderID,
		txHash:         event.TransactionHash,
		blockNumber:    event.BlockNumber,
		logIndex:       event.LogIndex,
		occurredAt:     event.RolledBackAt,
		postings: func(target *domain.WorkOrderBookkeepingTarget) []domain.LedgerPosting {
			return []domain.LedgerPosting{
				{AgentID: target.PayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: target.PayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeCredit},
				{AgentID: target.PayeeID, AccountName: domain.AccountNameServiceRevenue, AccountType: domain.AccountTypeRevenue, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: target.PayerID, AccountName: domain.AccountNameServiceExpense, AccountType: domain.AccountTypeExpense, EntryType: domain.LedgerEntryTypeCredit},
			}
		},
	},
		func(ctx context.Context) (*domain.WorkOrderBookkeepingTarget, bool, error) {
			return uc.workOrderRepository.ResolveOrderReleasedRollbackTarget(ctx, event)
		},
		func(ctx context.Context, entry domain.EscrowJournalEntry) (bool, error) {
			return uc.workOrderRepository.ApplyOrderReleasedRollback(ctx, event, entry)
		},
	)
}

func (uc *WorkOrderUsecase) RecordOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (bool, error) {
	event.RecordedAt = event.RecordedAt.UTC()

	return uc.recordEscrowEvent(ctx, escrowEventContext{
		op:             "record order refunded",
		action:         "order_refunded",
		description:    fmt.Sprintf("Escrow order refunded for on-chain order %s", event.OnchainOrderID.String()),
		amount:         event.Amount,
		onchainOrderID: event.OnchainOrderID,
		txHash:         event.TransactionHash,
		blockNumber:    event.BlockNumber,
		logIndex:       event.LogIndex,
		occurredAt:     event.RecordedAt,
		postings: func(target *domain.WorkOrderBookkeepingTarget) []domain.LedgerPosting {
			return []domain.LedgerPosting{
				{AgentID: target.PayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: target.PayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeCredit},
			}
		},
	},
		func(ctx context.Context) (*domain.WorkOrderBookkeepingTarget, bool, error) {
			return uc.workOrderRepository.ResolveOrderRefundedTarget(ctx, event)
		},
		func(ctx context.Context, entry domain.EscrowJournalEntry) (bool, error) {
			return uc.workOrderRepository.ApplyOrderRefunded(ctx, event, entry)
		},
	)
}

func (uc *WorkOrderUsecase) RollbackOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (bool, error) {
	event.RolledBackAt = event.RolledBackAt.UTC()

	return uc.recordEscrowEvent(ctx, escrowEventContext{
		op:             "rollback order refunded",
		action:         "order_refunded_rollback",
		description:    fmt.Sprintf("Escrow order refunded rollback for on-chain order %s", event.OnchainOrderID.String()),
		amount:         event.Amount,
		onchainOrderID: event.OnchainOrderID,
		txHash:         event.TransactionHash,
		blockNumber:    event.BlockNumber,
		logIndex:       event.LogIndex,
		occurredAt:     event.RolledBackAt,
		postings: func(target *domain.WorkOrderBookkeepingTarget) []domain.LedgerPosting {
			return []domain.LedgerPosting{
				{AgentID: target.PayerID, AccountName: accountNameEscrowLocked, AccountType: domain.AccountTypeAsset, EntryType: domain.LedgerEntryTypeDebit},
				{AgentID: target.PayeeID, AccountName: accountNameEscrowPending, AccountType: domain.AccountTypeLiability, EntryType: domain.LedgerEntryTypeCredit},
			}
		},
	},
		func(ctx context.Context) (*domain.WorkOrderBookkeepingTarget, bool, error) {
			return uc.workOrderRepository.ResolveOrderRefundedRollbackTarget(ctx, event)
		},
		func(ctx context.Context, entry domain.EscrowJournalEntry) (bool, error) {
			return uc.workOrderRepository.ApplyOrderRefundedRollback(ctx, event, entry)
		},
	)
}

// escrowEventContext carries everything that varies between the six escrow events
// so recordEscrowEvent can orchestrate them uniformly.
type escrowEventContext struct {
	op             string // human-readable operation, used in error wrapping
	action         string // stable token embedded in the journal idempotency key
	description    string
	amount         big.Int
	onchainOrderID big.Int
	txHash         string
	blockNumber    string
	logIndex       string
	occurredAt     time.Time
	postings       func(target *domain.WorkOrderBookkeepingTarget) []domain.LedgerPosting
}

// recordEscrowEvent runs the escrow bookkeeping orchestration for one on-chain
// event: resolve the target (read-only), upload the journal anchor to 0G with no
// DB transaction held (M1), then persist the state transition + journal/ledger
// atomically. A non-matching resolve is a no-op — no upload, no mutation.
func (uc *WorkOrderUsecase) recordEscrowEvent(
	ctx context.Context,
	evt escrowEventContext,
	resolve func(context.Context) (*domain.WorkOrderBookkeepingTarget, bool, error),
	apply func(context.Context, domain.EscrowJournalEntry) (bool, error),
) (bool, error) {
	target, matched, err := resolve(ctx)
	if err != nil {
		return false, fmt.Errorf("%w: %s: %w", domain.ErrPersistence, evt.op, err)
	}
	if !matched {
		return false, nil
	}

	journalID, err := uc.newID()
	if err != nil {
		return false, fmt.Errorf("%s: generate journal entry id: %w", evt.op, err)
	}

	entry := domain.EscrowJournalEntry{
		JournalID:      journalID,
		IdempotencyKey: escrowJournalKey(target.WorkOrderID, evt.action, evt.txHash, evt.blockNumber, evt.logIndex, evt.onchainOrderID),
		WorkOrderID:    target.WorkOrderID,
		Description:    evt.description,
		CreatedAt:      evt.occurredAt,
		Amount:         evt.amount,
		Postings:       evt.postings(target),
	}

	storageCID, err := uc.uploadEscrowJournal(ctx, entry)
	if err != nil {
		return false, fmt.Errorf("%w: %s: %w", domain.ErrStorage, evt.op, err)
	}
	entry.StorageCID = storageCID

	updated, err := apply(ctx, entry)
	if err != nil {
		return false, fmt.Errorf("%w: %s: %w", domain.ErrPersistence, evt.op, err)
	}

	return updated, nil
}

func (uc *WorkOrderUsecase) uploadEscrowJournal(ctx context.Context, entry domain.EscrowJournalEntry) (string, error) {
	output, err := uc.zgStorage.UploadJSON(ctx, escrowJournalAnchor(entry))
	if err != nil {
		return "", fmt.Errorf("upload escrow journal entry: %w", err)
	}
	if output == nil || strings.TrimSpace(output.RootHash) == "" {
		return "", errors.New("upload escrow journal entry returned empty root hash")
	}

	return output.RootHash, nil
}

// bookkeepingJournalAnchor is the canonical-JSON payload pinned to 0G and hashed
// for on-chain auditability. Its schema and JSON tags are part of that commitment
// and must not change without versioning.
type bookkeepingJournalAnchor struct {
	SchemaVersion  string                            `json:"schema_version"`
	Kind           string                            `json:"kind"`
	JournalEntryID uuid.UUID                         `json:"journal_entry_id"`
	IdempotencyKey string                            `json:"idempotency_key"`
	WorkOrderID    uuid.UUID                         `json:"work_order_id"`
	Description    string                            `json:"description"`
	CreatedAt      string                            `json:"created_at"`
	Postings       []bookkeepingJournalAnchorPosting `json:"postings"`
}

type bookkeepingJournalAnchorPosting struct {
	AccountName string                 `json:"account_name"`
	AccountType domain.AccountType     `json:"account_type"`
	EntryType   domain.LedgerEntryType `json:"entry_type"`
	Amount      string                 `json:"amount"`
	AgentID     uuid.UUID              `json:"agent_id"`
}

func escrowJournalAnchor(entry domain.EscrowJournalEntry) bookkeepingJournalAnchor {
	payload := bookkeepingJournalAnchor{
		SchemaVersion:  "1.0",
		Kind:           "escrow_journal_entry",
		JournalEntryID: entry.JournalID,
		IdempotencyKey: entry.IdempotencyKey,
		WorkOrderID:    entry.WorkOrderID,
		Description:    entry.Description,
		CreatedAt:      entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		Postings:       make([]bookkeepingJournalAnchorPosting, 0, len(entry.Postings)),
	}
	amount := entry.Amount.String()
	for _, posting := range entry.Postings {
		payload.Postings = append(payload.Postings, bookkeepingJournalAnchorPosting{
			AccountName: posting.AccountName,
			AccountType: posting.AccountType,
			EntryType:   posting.EntryType,
			Amount:      amount,
			AgentID:     posting.AgentID,
		})
	}

	return payload
}

func escrowJournalKey(workOrderID uuid.UUID, action string, transactionHash string, blockNumber string, logIndex string, onchainOrderID big.Int) string {
	eventID := strings.TrimSpace(transactionHash)
	if eventID != "" {
		blockNumber = strings.TrimSpace(blockNumber)
		logIndex = strings.TrimSpace(logIndex)
		if blockNumber != "" || logIndex != "" {
			eventID = fmt.Sprintf("%s:%s:%s", eventID, blockNumber, logIndex)
		}
	} else {
		eventID = onchainOrderID.String()
	}

	return fmt.Sprintf("work_order:%s:%s:%s", workOrderID, action, eventID)
}

func validateWorkOrderSpecRequest(request domain.WorkOrderSpecRequest) error {
	if isBlank(request.IdempotencyKey) {
		return domain.NewValidationError("idempotency_key is required")
	}
	if isBlank(request.Parties.Payer) {
		return domain.NewValidationError("parties.payer is required")
	}
	if isBlank(request.Parties.Payee) {
		return domain.NewValidationError("parties.payee is required")
	}
	if isBlank(request.Task.Title) {
		return domain.NewValidationError("task.title is required")
	}
	if isBlank(request.Task.Description) {
		return domain.NewValidationError("task.description is required")
	}
	if isBlank(request.Task.Category) {
		return domain.NewValidationError("task.category is required")
	}
	if isBlank(request.Deliverable.Format) {
		return domain.NewValidationError("deliverable.format is required")
	}
	if isBlank(request.Deliverable.Submission.Method) {
		return domain.NewValidationError("deliverable.submission.method is required")
	}
	if isBlank(request.Deliverable.Submission.Endpoint) {
		return domain.NewValidationError("deliverable.submission.endpoint is required")
	}
	if len(request.AcceptanceCriteria) == 0 {
		return domain.NewValidationError("acceptanceCriteria must contain at least one item")
	}
	for i, criteria := range request.AcceptanceCriteria {
		if isBlank(criteria.ID) {
			return domain.NewValidationError("acceptanceCriteria[%d].id is required", i)
		}
		if isBlank(criteria.Description) {
			return domain.NewValidationError("acceptanceCriteria[%d].description is required", i)
		}
	}
	if isBlank(request.Compensation.Amount) {
		return domain.NewValidationError("compensation.amount is required")
	}
	if isBlank(request.Compensation.Asset) {
		return domain.NewValidationError("compensation.asset is required")
	}
	if isBlank(request.Compensation.Chain) {
		return domain.NewValidationError("compensation.chain is required")
	}
	if isBlank(request.Deadline) {
		return domain.NewValidationError("deadline is required")
	}
	if _, err := time.Parse(time.RFC3339, request.Deadline); err != nil {
		return domain.NewValidationError("deadline must be a valid RFC3339 timestamp")
	}

	return nil
}

func validateWorkOrderDeliveryRequest(request domain.WorkOrderDeliveryRequest) error {
	if isBlank(request.DeliveryHash) {
		return domain.NewValidationError("deliveryHash is required")
	}
	if !hex32Pattern.MatchString(request.DeliveryHash) {
		return domain.NewValidationError("deliveryHash must be a 0x-prefixed 32-byte hex string")
	}
	if isBlank(request.Signature) {
		return domain.NewValidationError("signature is required")
	}
	if !strings.HasPrefix(request.Signature, "0x") && !strings.HasPrefix(request.Signature, "0X") {
		return domain.NewValidationError("signature must be a 0x-prefixed 65-byte hex string")
	}
	signatureBytes, err := hex.DecodeString(trimHexPrefix(request.Signature))
	if err != nil || len(signatureBytes) != 65 {
		return domain.NewValidationError("signature must be a 0x-prefixed 65-byte hex string")
	}

	return nil
}

func (uc *WorkOrderUsecase) findAgentByWallet(ctx context.Context, fieldName string, walletAddress string) (*domain.Agent, error) {
	agent, err := uc.agentRepository.FindByWalletAddress(ctx, walletAddress)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.NewValidationError("%s references an unknown agent wallet", fieldName)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: find agent for %s: %w", domain.ErrPersistence, fieldName, err)
	}

	return agent, nil
}

func parseWorkOrderAmount(value string) (big.Int, error) {
	amount := new(big.Int)
	if _, ok := amount.SetString(strings.TrimSpace(value), 10); !ok || amount.Sign() <= 0 {
		return big.Int{}, domain.NewValidationError("compensation.amount must be a positive integer")
	}

	return *amount, nil
}

func parseOnchainOrderID(value string) (big.Int, string, error) {
	trimmed := strings.TrimSpace(value)
	orderID := new(big.Int)
	if trimmed == "" {
		return big.Int{}, "", domain.NewValidationError("onchainOrderID is required")
	}
	if _, ok := orderID.SetString(trimmed, 10); !ok || orderID.Sign() < 0 {
		return big.Int{}, "", domain.NewValidationError("onchainOrderID must be a non-negative integer")
	}

	return *orderID, trimmed, nil
}

func recoverEthereumAddress(message string, signature string) (string, error) {
	signatureBytes, err := hex.DecodeString(trimHexPrefix(signature))
	if err != nil {
		return "", err
	}
	if len(signatureBytes) != 65 {
		return "", fmt.Errorf("invalid signature length: %d", len(signatureBytes))
	}

	switch signatureBytes[64] {
	case 27, 28:
		signatureBytes[64] -= 27
	case 0, 1:
	default:
		return "", fmt.Errorf("invalid signature recovery id")
	}

	publicKey, err := crypto.SigToPub(accounts.TextHash([]byte(message)), signatureBytes)
	if err != nil {
		return "", err
	}

	return crypto.PubkeyToAddress(*publicKey).Hex(), nil
}

func (uc *WorkOrderUsecase) classifyDeliveryUpdateConflict(ctx context.Context, onchainOrderID big.Int) error {
	target, err := uc.workOrderRepository.FindDeliveryTargetByOnchainOrderID(ctx, onchainOrderID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("%w: refresh delivery target: %w", domain.ErrPersistence, err)
	}
	if target.WorkOrder.Status != domain.WorkOrderStatusFunded {
		return domain.ErrOrderNotFunded
	}
	if target.WorkOrder.DeliverableCID != nil {
		return domain.ErrDeliveryAlreadyPosted
	}

	return domain.ErrDeliveryAlreadyPosted
}

func workOrderSpecResponseFromWorkOrder(workOrder *domain.WorkOrder) *domain.WorkOrderSpecResponse {
	return &domain.WorkOrderSpecResponse{
		ID:       workOrder.ID,
		RootHash: workOrder.SpecHash,
		TxHash:   workOrder.SpecTxHash,
	}
}

func isBlank(value string) bool {
	return strings.TrimSpace(value) == ""
}

func trimHexPrefix(value string) string {
	return strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
}
