package domain

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type WorkOrderSpecRequest struct {
	IdempotencyKey     string                            `json:"idempotency_key"`
	Parties            WorkOrderSpecParties              `json:"parties"`
	Task               WorkOrderSpecTask                 `json:"task"`
	Deliverable        WorkOrderSpecDeliverable          `json:"deliverable"`
	AcceptanceCriteria []WorkOrderSpecAcceptanceCriteria `json:"acceptanceCriteria"`
	Compensation       WorkOrderSpecCompensation         `json:"compensation"`
	Deadline           string                            `json:"deadline"`
}

type WorkOrderSpec struct {
	Version            string                            `json:"version"`
	ID                 uuid.UUID                         `json:"id"`
	CreatedAt          string                            `json:"createdAt"`
	Parties            WorkOrderSpecParties              `json:"parties"`
	Task               WorkOrderSpecTask                 `json:"task"`
	Deliverable        WorkOrderSpecDeliverable          `json:"deliverable"`
	AcceptanceCriteria []WorkOrderSpecAcceptanceCriteria `json:"acceptanceCriteria"`
	Compensation       WorkOrderSpecCompensation         `json:"compensation"`
	Deadline           string                            `json:"deadline"`
}

type WorkOrderSpecParties struct {
	Payer string `json:"payer"`
	Payee string `json:"payee"`
}

type WorkOrderSpecTask struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type WorkOrderSpecDeliverable struct {
	Format     string                  `json:"format"`
	Submission WorkOrderSpecSubmission `json:"submission"`
}

type WorkOrderSpecSubmission struct {
	Method   string `json:"method"`
	Endpoint string `json:"endpoint"`
}

type WorkOrderSpecAcceptanceCriteria struct {
	ID               string  `json:"id"`
	Description      string  `json:"description"`
	VerificationHint *string `json:"verificationHint"`
}

type WorkOrderSpecCompensation struct {
	Amount string `json:"amount"`
	Asset  string `json:"asset"`
	Chain  string `json:"chain"`
}

type WorkOrderSpecResponse struct {
	ID       uuid.UUID `json:"id"`
	RootHash string    `json:"root_hash"`
	TxHash   string    `json:"tx_hash"`
}

type WorkOrderUsecase interface {
	UploadSpec(ctx context.Context, request WorkOrderSpecRequest) (*WorkOrderSpecResponse, error)
}

type ValidationError struct {
	Message string
}

func NewValidationError(format string, args ...any) *ValidationError {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

func (e *ValidationError) Error() string {
	return e.Message
}
