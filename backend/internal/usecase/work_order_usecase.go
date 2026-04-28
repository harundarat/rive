package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

const workOrderSpecVersion = "1.0"

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

func (uc *WorkOrderUsecase) RecordOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (bool, error) {
	event.RecordedAt = event.RecordedAt.UTC()

	updated, err := uc.workOrderRepository.RecordOrderCreated(ctx, event)
	if err != nil {
		return false, fmt.Errorf("%w: record order created: %w", domain.ErrPersistence, err)
	}

	return updated, nil
}

func (uc *WorkOrderUsecase) RollbackOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (bool, error) {
	event.RolledBackAt = event.RolledBackAt.UTC()

	updated, err := uc.workOrderRepository.RollbackOrderCreated(ctx, event)
	if err != nil {
		return false, fmt.Errorf("%w: rollback order created: %w", domain.ErrPersistence, err)
	}

	return updated, nil
}

func (uc *WorkOrderUsecase) RecordOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (bool, error) {
	event.RecordedAt = event.RecordedAt.UTC()

	updated, err := uc.workOrderRepository.RecordOrderReleased(ctx, event)
	if err != nil {
		return false, fmt.Errorf("%w: record order released: %w", domain.ErrPersistence, err)
	}

	return updated, nil
}

func (uc *WorkOrderUsecase) RollbackOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (bool, error) {
	event.RolledBackAt = event.RolledBackAt.UTC()

	updated, err := uc.workOrderRepository.RollbackOrderReleased(ctx, event)
	if err != nil {
		return false, fmt.Errorf("%w: rollback order released: %w", domain.ErrPersistence, err)
	}

	return updated, nil
}

func (uc *WorkOrderUsecase) RecordOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (bool, error) {
	event.RecordedAt = event.RecordedAt.UTC()

	updated, err := uc.workOrderRepository.RecordOrderRefunded(ctx, event)
	if err != nil {
		return false, fmt.Errorf("%w: record order refunded: %w", domain.ErrPersistence, err)
	}

	return updated, nil
}

func (uc *WorkOrderUsecase) RollbackOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (bool, error) {
	event.RolledBackAt = event.RolledBackAt.UTC()

	updated, err := uc.workOrderRepository.RollbackOrderRefunded(ctx, event)
	if err != nil {
		return false, fmt.Errorf("%w: rollback order refunded: %w", domain.ErrPersistence, err)
	}

	return updated, nil
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
