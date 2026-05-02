package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

type fakeNettingUsecase struct {
	input  domain.PaymentIntentRequest
	output *domain.PaymentIntentResponse
	err    error
	calls  int
}

func (uc *fakeNettingUsecase) SubmitIntent(ctx context.Context, request domain.PaymentIntentRequest) (*domain.PaymentIntentResponse, error) {
	uc.calls++
	uc.input = request
	if uc.err != nil {
		return nil, uc.err
	}

	return uc.output, nil
}

func TestNettingHandlerCreatePaymentIntentSuccess(t *testing.T) {
	intentID := uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a1f")
	usecase := &fakeNettingUsecase{
		output: &domain.PaymentIntentResponse{
			ID:             intentID,
			IdempotencyKey: "intent-1",
			Payer:          "0x1111111111111111111111111111111111111111",
			Payee:          "0x2222222222222222222222222222222222222222",
			Amount:         "1000000",
			Asset:          "rUSD",
			Status:         "pending",
			CreatedAt:      "2026-05-02T10:30:00Z",
			UpdatedAt:      "2026-05-02T10:30:00Z",
		},
	}
	handler := NewNettingHandler(usecase)
	req := httptest.NewRequest(nethttp.MethodPost, "/api/payments/intent", bytes.NewBufferString(validPaymentIntentJSON()))
	rec := httptest.NewRecorder()

	handler.CreatePaymentIntent(rec, req)

	if rec.Code != nethttp.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if usecase.input.IdempotencyKey != "intent-1" {
		t.Fatalf("expected decoded idempotency key, got %q", usecase.input.IdempotencyKey)
	}

	var body struct {
		Success bool                         `json:"success"`
		Data    domain.PaymentIntentResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success {
		t.Fatal("expected success response")
	}
	if body.Data.ID != intentID {
		t.Fatalf("expected intent id %s, got %s", intentID, body.Data.ID)
	}
}

func TestNettingHandlerCreatePaymentIntentInvalidJSON(t *testing.T) {
	handler := NewNettingHandler(&fakeNettingUsecase{})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/payments/intent", bytes.NewBufferString("{"))
	rec := httptest.NewRecorder()

	handler.CreatePaymentIntent(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestNettingHandlerCreatePaymentIntentErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "validation", err: domain.NewValidationError("amount is required"), statusCode: nethttp.StatusBadRequest, code: "BAD_REQUEST"},
		{name: "unsupported asset", err: domain.ErrUnsupportedAsset, statusCode: nethttp.StatusBadRequest, code: "UNSUPPORTED_ASSET"},
		{name: "persistence", err: errors.Join(domain.ErrPersistence, errors.New("insert failed")), statusCode: nethttp.StatusInternalServerError, code: "FAILED_TO_CREATE_PAYMENT_INTENT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewNettingHandler(&fakeNettingUsecase{err: tt.err})
			req := httptest.NewRequest(nethttp.MethodPost, "/api/payments/intent", bytes.NewBufferString(validPaymentIntentJSON()))
			rec := httptest.NewRecorder()

			handler.CreatePaymentIntent(rec, req)

			if rec.Code != tt.statusCode {
				t.Fatalf("expected status %d, got %d: %s", tt.statusCode, rec.Code, rec.Body.String())
			}

			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if body.Error.Code != tt.code {
				t.Fatalf("expected error code %q, got %q", tt.code, body.Error.Code)
			}
		})
	}
}

func validPaymentIntentJSON() string {
	return `{
		"idempotency_key": "intent-1",
		"payer": "0x1111111111111111111111111111111111111111",
		"payee": "0x2222222222222222222222222222222222222222",
		"amount": "1000000",
		"asset": "rUSD"
	}`
}
