package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

const workOrderSpecVersion = "1.0"

type WorkOrderUsecase struct {
	zgStorage domain.ZGStorage
	now       func() time.Time
	newID     func() (uuid.UUID, error)
}

func NewWorkOrderUsecase(zgStorage domain.ZGStorage) *WorkOrderUsecase {
	return &WorkOrderUsecase{
		zgStorage: zgStorage,
		now:       time.Now,
		newID:     uuid.NewV7,
	}
}

func (uc *WorkOrderUsecase) UploadSpec(ctx context.Context, input domain.WorkOrderSpecInput) (*domain.WorkOrderSpecUploadOutput, error) {
	if err := validateWorkOrderSpecInput(input); err != nil {
		return nil, err
	}

	id, err := uc.newID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate work order id: %w", err)
	}

	spec := domain.WorkOrderSpec{
		Version:            workOrderSpecVersion,
		ID:                 "wo_" + id.String(),
		CreatedAt:          uc.now().UTC().Format(time.RFC3339),
		Parties:            input.Parties,
		Task:               input.Task,
		Deliverable:        input.Deliverable,
		AcceptanceCriteria: input.AcceptanceCriteria,
		Compensation:       input.Compensation,
		Deadline:           input.Deadline,
	}

	uploadOutput, err := uc.zgStorage.UploadJSON(ctx, spec)
	if err != nil {
		return nil, err
	}

	return &domain.WorkOrderSpecUploadOutput{
		ID:       spec.ID,
		RootHash: uploadOutput.RootHash,
		TxHash:   uploadOutput.TxHash,
	}, nil
}

func validateWorkOrderSpecInput(input domain.WorkOrderSpecInput) error {
	if isBlank(input.Parties.Payer) {
		return domain.NewValidationError("parties.payer is required")
	}
	if isBlank(input.Parties.Payee) {
		return domain.NewValidationError("parties.payee is required")
	}
	if isBlank(input.Task.Title) {
		return domain.NewValidationError("task.title is required")
	}
	if isBlank(input.Task.Description) {
		return domain.NewValidationError("task.description is required")
	}
	if isBlank(input.Task.Category) {
		return domain.NewValidationError("task.category is required")
	}
	if isBlank(input.Deliverable.Format) {
		return domain.NewValidationError("deliverable.format is required")
	}
	if isBlank(input.Deliverable.Submission.Method) {
		return domain.NewValidationError("deliverable.submission.method is required")
	}
	if isBlank(input.Deliverable.Submission.Endpoint) {
		return domain.NewValidationError("deliverable.submission.endpoint is required")
	}
	if len(input.AcceptanceCriteria) == 0 {
		return domain.NewValidationError("acceptanceCriteria must contain at least one item")
	}
	for i, criteria := range input.AcceptanceCriteria {
		if isBlank(criteria.ID) {
			return domain.NewValidationError("acceptanceCriteria[%d].id is required", i)
		}
		if isBlank(criteria.Description) {
			return domain.NewValidationError("acceptanceCriteria[%d].description is required", i)
		}
	}
	if isBlank(input.Compensation.Amount) {
		return domain.NewValidationError("compensation.amount is required")
	}
	if isBlank(input.Compensation.Asset) {
		return domain.NewValidationError("compensation.asset is required")
	}
	if isBlank(input.Compensation.Chain) {
		return domain.NewValidationError("compensation.chain is required")
	}
	if isBlank(input.Deadline) {
		return domain.NewValidationError("deadline is required")
	}
	if _, err := time.Parse(time.RFC3339, input.Deadline); err != nil {
		return domain.NewValidationError("deadline must be a valid RFC3339 timestamp")
	}

	return nil
}

func isBlank(value string) bool {
	return strings.TrimSpace(value) == ""
}
