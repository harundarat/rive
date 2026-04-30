package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

type fakeWorkOrderUsecase struct {
	input               domain.WorkOrderSpecRequest
	output              *domain.WorkOrderSpecResponse
	err                 error
	deliveryOrderID     string
	deliveryInput       domain.WorkOrderDeliveryRequest
	deliveryOutput      *domain.WorkOrderDeliveryResponse
	deliveryErr         error
	submitDeliveryCalls int
}

func (uc *fakeWorkOrderUsecase) UploadSpec(ctx context.Context, request domain.WorkOrderSpecRequest) (*domain.WorkOrderSpecResponse, error) {
	uc.input = request
	if uc.err != nil {
		return nil, uc.err
	}

	return uc.output, nil
}

func (uc *fakeWorkOrderUsecase) SubmitDelivery(ctx context.Context, onchainOrderID string, request domain.WorkOrderDeliveryRequest) (*domain.WorkOrderDeliveryResponse, error) {
	uc.submitDeliveryCalls++
	uc.deliveryOrderID = onchainOrderID
	uc.deliveryInput = request
	if uc.deliveryErr != nil {
		return nil, uc.deliveryErr
	}

	return uc.deliveryOutput, nil
}

func TestWorkOrderHandlerCreateSuccess(t *testing.T) {
	usecase := &fakeWorkOrderUsecase{
		output: &domain.WorkOrderSpecResponse{
			ID:       uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a1f"),
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
		Success bool                         `json:"success"`
		Data    domain.WorkOrderSpecResponse `json:"data"`
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

func TestWorkOrderHandlerSubmitDeliverySuccess(t *testing.T) {
	deliveredAt := time.Date(2026, 4, 22, 10, 30, 0, 0, time.UTC).Format(time.RFC3339)
	usecase := &fakeWorkOrderUsecase{
		deliveryOutput: &domain.WorkOrderDeliveryResponse{
			OnchainOrderID: "123",
			DeliverableCID: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			DeliveredAt:    deliveredAt,
		},
	}
	handler := NewWorkOrderHandler(usecase)
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders/123/delivery", bytes.NewBufferString(validWorkOrderDeliveryJSON()))
	rec := httptest.NewRecorder()

	serveSubmitDelivery(handler, rec, req)

	if rec.Code != nethttp.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if usecase.deliveryOrderID != "123" {
		t.Fatalf("expected path order id 123, got %q", usecase.deliveryOrderID)
	}
	if usecase.deliveryInput.DeliveryHash != "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected decoded delivery hash, got %q", usecase.deliveryInput.DeliveryHash)
	}

	var body struct {
		Success bool                             `json:"success"`
		Data    domain.WorkOrderDeliveryResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success {
		t.Fatal("expected success response")
	}
	if body.Data.DeliveredAt != deliveredAt {
		t.Fatalf("expected delivered_at in response, got %q", body.Data.DeliveredAt)
	}
}

func TestWorkOrderHandlerSubmitDeliveryInvalidJSON(t *testing.T) {
	handler := NewWorkOrderHandler(&fakeWorkOrderUsecase{})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders/123/delivery", bytes.NewBufferString("{"))
	rec := httptest.NewRecorder()

	serveSubmitDelivery(handler, rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestWorkOrderHandlerSubmitDeliveryErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "validation", err: domain.NewValidationError("deliveryHash is required"), statusCode: nethttp.StatusBadRequest, code: "BAD_REQUEST"},
		{name: "invalid signature", err: domain.ErrInvalidSignature, statusCode: nethttp.StatusUnauthorized, code: "INVALID_SIGNATURE"},
		{name: "payee mismatch", err: domain.ErrPayeeMismatch, statusCode: nethttp.StatusForbidden, code: "PAYEE_MISMATCH"},
		{name: "not found", err: domain.ErrNotFound, statusCode: nethttp.StatusNotFound, code: "WORK_ORDER_NOT_FOUND"},
		{name: "not funded", err: domain.ErrOrderNotFunded, statusCode: nethttp.StatusConflict, code: "ORDER_NOT_FUNDED"},
		{name: "already posted", err: domain.ErrDeliveryAlreadyPosted, statusCode: nethttp.StatusConflict, code: "DELIVERY_ALREADY_POSTED"},
		{name: "persistence", err: errors.Join(domain.ErrPersistence, errors.New("update failed")), statusCode: nethttp.StatusInternalServerError, code: "FAILED_TO_SUBMIT_DELIVERY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewWorkOrderHandler(&fakeWorkOrderUsecase{deliveryErr: tt.err})
			req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders/123/delivery", bytes.NewBufferString(validWorkOrderDeliveryJSON()))
			rec := httptest.NewRecorder()

			serveSubmitDelivery(handler, rec, req)

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
	handler := NewWorkOrderHandler(&fakeWorkOrderUsecase{err: errors.Join(domain.ErrStorage, errors.New("upload failed"))})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders", bytes.NewBufferString(validWorkOrderSpecJSON()))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != nethttp.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestWorkOrderHandlerCreatePersistenceError(t *testing.T) {
	handler := NewWorkOrderHandler(&fakeWorkOrderUsecase{err: errors.Join(domain.ErrPersistence, errors.New("create failed"))})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/work-orders", bytes.NewBufferString(validWorkOrderSpecJSON()))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != nethttp.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Error.Code != "FAILED_TO_CREATE_WORK_ORDER" {
		t.Fatalf("expected persistence error code, got %q", body.Error.Code)
	}
}

func serveSubmitDelivery(handler *WorkOrderHandler, rec *httptest.ResponseRecorder, req *nethttp.Request) {
	router := chi.NewRouter()
	router.Post("/api/work-orders/{onchainOrderID}/delivery", handler.SubmitDelivery)
	router.ServeHTTP(rec, req)
}

func validWorkOrderSpecJSON() string {
	return `{
		"idempotency_key": "wo-request-1",
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

func validWorkOrderDeliveryJSON() string {
	return `{
		"deliveryHash": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"signature": "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	}`
}
