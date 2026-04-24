package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/harundarat/rive/backend/internal/domain"
)

type fakeWorkOrderUsecase struct {
	input  domain.WorkOrderSpecInput
	output *domain.WorkOrderSpecUploadOutput
	err    error
}

func (uc *fakeWorkOrderUsecase) UploadSpec(ctx context.Context, input domain.WorkOrderSpecInput) (*domain.WorkOrderSpecUploadOutput, error) {
	uc.input = input
	if uc.err != nil {
		return nil, uc.err
	}

	return uc.output, nil
}

func TestWorkOrderHandlerCreateSuccess(t *testing.T) {
	usecase := &fakeWorkOrderUsecase{
		output: &domain.WorkOrderSpecUploadOutput{
			ID:       "wo_018f95e4-3f8d-7b70-a4dd-2d9a833c4a1f",
			RootHash: "0xroot",
			TxHash:   "0xtx",
		},
	}
	handler := NewWorkOrderHandler(usecase)
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders", bytes.NewBufferString(validWorkOrderSpecJSON()))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != nethttp.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if usecase.input.Task.Title != "Scrape and clean Yelp reviews for restaurant XYZ" {
		t.Fatalf("expected decoded task title, got %q", usecase.input.Task.Title)
	}

	var body struct {
		Success bool                             `json:"success"`
		Data    domain.WorkOrderSpecUploadOutput `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success {
		t.Fatal("expected success response")
	}
	if body.Data.RootHash != "0xroot" {
		t.Fatalf("expected root hash in response, got %q", body.Data.RootHash)
	}
}

func TestWorkOrderHandlerCreateInvalidJSON(t *testing.T) {
	handler := NewWorkOrderHandler(&fakeWorkOrderUsecase{})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders", bytes.NewBufferString("{"))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestWorkOrderHandlerCreateNonObjectJSON(t *testing.T) {
	handler := NewWorkOrderHandler(&fakeWorkOrderUsecase{})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders", bytes.NewBufferString("[]"))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestWorkOrderHandlerCreateValidationError(t *testing.T) {
	handler := NewWorkOrderHandler(&fakeWorkOrderUsecase{err: domain.NewValidationError("task.title is required")})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders", bytes.NewBufferString(validWorkOrderSpecJSON()))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestWorkOrderHandlerCreateUploadError(t *testing.T) {
	handler := NewWorkOrderHandler(&fakeWorkOrderUsecase{err: errors.New("upload failed")})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders", bytes.NewBufferString(validWorkOrderSpecJSON()))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != nethttp.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func validWorkOrderSpecJSON() string {
	return `{
		"parties": {
			"payer": "0xabc",
			"payee": "0xdef"
		},
		"task": {
			"title": "Scrape and clean Yelp reviews for restaurant XYZ",
			"description": "Detailed prose deskripsi tugas...",
			"category": "data-extraction"
		},
		"deliverable": {
			"format": "json",
			"submission": {
				"method": "http-callback",
				"endpoint": "https://payee.example/deliver"
			}
		},
		"acceptanceCriteria": [
			{
				"id": "ac1",
				"description": "Output is valid JSON with >= 100 review objects",
				"verificationHint": null
			}
		],
		"compensation": {
			"amount": "10000000000000000000",
			"asset": "0G",
			"chain": "0g-mainnet"
		},
		"deadline": "2026-04-23T10:30:00Z"
	}`
}
