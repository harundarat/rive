package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

type fakeZGStorage struct {
	data any
	err  error
}

func (s *fakeZGStorage) UploadJSON(ctx context.Context, data any) (*domain.ZGUploadOutput, error) {
	s.data = data
	if s.err != nil {
		return nil, s.err
	}

	return &domain.ZGUploadOutput{RootHash: "0xroot", TxHash: "0xtx"}, nil
}

func TestWorkOrderUsecaseUploadSpecStoresFinalSchema(t *testing.T) {
	storage := &fakeZGStorage{}
	fixedID := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a1f")
	fixedTime := time.Date(2026, 4, 22, 10, 30, 0, 0, time.UTC)
	uc := &WorkOrderUsecase{
		zgStorage: storage,
		now:       func() time.Time { return fixedTime },
		newID:     func() (uuid.UUID, error) { return fixedID, nil },
	}

	output, err := uc.UploadSpec(context.Background(), validWorkOrderSpecInput())
	if err != nil {
		t.Fatalf("UploadSpec returned error: %v", err)
	}

	if output.ID != "wo_"+fixedID.String() {
		t.Fatalf("expected generated id, got %q", output.ID)
	}
	if output.RootHash != "0xroot" {
		t.Fatalf("expected root hash from storage, got %q", output.RootHash)
	}
	if output.TxHash != "0xtx" {
		t.Fatalf("expected tx hash from storage, got %q", output.TxHash)
	}

	spec, ok := storage.data.(domain.WorkOrderSpec)
	if !ok {
		t.Fatalf("expected storage data to be WorkOrderSpec, got %T", storage.data)
	}

	if spec.Version != "1.0" {
		t.Fatalf("expected version 1.0, got %q", spec.Version)
	}
	if spec.ID != "wo_"+fixedID.String() {
		t.Fatalf("expected generated spec id, got %q", spec.ID)
	}
	if spec.CreatedAt != "2026-04-22T10:30:00Z" {
		t.Fatalf("expected generated createdAt, got %q", spec.CreatedAt)
	}
	if spec.Parties.Payer != "0xabc" || spec.Parties.Payee != "0xdef" {
		t.Fatalf("expected parties to be preserved, got %+v", spec.Parties)
	}
	if spec.Task.Title != "Scrape and clean Yelp reviews for restaurant XYZ" {
		t.Fatalf("expected task title to be preserved, got %q", spec.Task.Title)
	}
	if spec.AcceptanceCriteria[0].VerificationHint != nil {
		t.Fatalf("expected nil verificationHint, got %q", *spec.AcceptanceCriteria[0].VerificationHint)
	}
	if spec.Deadline != "2026-04-23T10:30:00Z" {
		t.Fatalf("expected deadline to be preserved, got %q", spec.Deadline)
	}
}

func TestWorkOrderUsecaseUploadSpecValidation(t *testing.T) {
	tests := []struct {
		name  string
		input domain.WorkOrderSpecInput
	}{
		{
			name: "missing required field",
			input: func() domain.WorkOrderSpecInput {
				input := validWorkOrderSpecInput()
				input.Task.Title = ""
				return input
			}(),
		},
		{
			name: "invalid deadline",
			input: func() domain.WorkOrderSpecInput {
				input := validWorkOrderSpecInput()
				input.Deadline = "tomorrow"
				return input
			}(),
		},
		{
			name: "empty acceptance criteria",
			input: func() domain.WorkOrderSpecInput {
				input := validWorkOrderSpecInput()
				input.AcceptanceCriteria = nil
				return input
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewWorkOrderUsecase(&fakeZGStorage{})

			_, err := uc.UploadSpec(context.Background(), tt.input)
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

func TestWorkOrderUsecaseUploadSpecReturnsStorageError(t *testing.T) {
	expectedErr := errors.New("storage unavailable")
	uc := NewWorkOrderUsecase(&fakeZGStorage{err: expectedErr})

	_, err := uc.UploadSpec(context.Background(), validWorkOrderSpecInput())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected storage error, got %v", err)
	}
}

func validWorkOrderSpecInput() domain.WorkOrderSpecInput {
	return domain.WorkOrderSpecInput{
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
