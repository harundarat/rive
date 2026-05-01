package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/harundarat/rive/backend/internal/domain"
)

type fakePnLUsecase struct {
	walletAddress string
	fromRaw       string
	toRaw         string
	output        *domain.PnLReport
	err           error
	calls         int
}

func (uc *fakePnLUsecase) GetPnL(ctx context.Context, walletAddress string, fromRaw string, toRaw string) (*domain.PnLReport, error) {
	uc.calls++
	uc.walletAddress = walletAddress
	uc.fromRaw = fromRaw
	uc.toRaw = toRaw
	if uc.err != nil {
		return nil, uc.err
	}

	return uc.output, nil
}

func TestLedgerHandlerGetPnLSuccess(t *testing.T) {
	from := "2026-04-01T00:00:00Z"
	usecase := &fakePnLUsecase{
		output: &domain.PnLReport{
			Agent: domain.PnLAgent{
				Address:      "0x26DEa28e89dFdF4CD5Ab9f63010bB46316EC3A73",
				RegisteredAt: "2026-04-15T08:00:00Z",
			},
			Period: domain.PnLPeriod{
				From: &from,
				To:   "2026-05-01T00:00:00Z",
			},
			Asset: domain.PnLAssetRUSD,
			Summary: domain.PnLSummary{
				TotalRevenue:     "150000000",
				TotalExpenses:    "45000000",
				NetIncome:        "105000000",
				TransactionCount: 20,
			},
			Revenue: []domain.PnLAccountSummary{
				{Account: "Service Revenue", Amount: "150000000", EntryCount: 15},
			},
			Expenses: []domain.PnLAccountSummary{
				{Account: "Service Expense", Amount: "45000000", EntryCount: 5},
			},
			AuditTrail: domain.PnLAuditTrail{
				JournalBatchCount: 0,
				Batches:           []domain.PnLAuditBatch{},
			},
			GeneratedAt: "2026-05-01T10:30:00Z",
			Version:     domain.PnLVersion,
		},
	}
	handler := NewLedgerHandler(usecase)
	req := httptest.NewRequest(nethttp.MethodGet, "/api/ledger/0xabc/pnl?from=2026-04-01T00:00:00Z&to=2026-05-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	serveGetPnL(handler, rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if usecase.walletAddress != "0xabc" {
		t.Fatalf("expected wallet path param, got %q", usecase.walletAddress)
	}
	if usecase.fromRaw != "2026-04-01T00:00:00Z" || usecase.toRaw != "2026-05-01T00:00:00Z" {
		t.Fatalf("expected query period, got from=%q to=%q", usecase.fromRaw, usecase.toRaw)
	}

	var body struct {
		Success bool             `json:"success"`
		Data    domain.PnLReport `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success {
		t.Fatal("expected success response")
	}
	if body.Data.Summary.NetIncome != "105000000" {
		t.Fatalf("expected net income, got %q", body.Data.Summary.NetIncome)
	}
	if len(body.Data.Revenue) != 1 || body.Data.Revenue[0].Account != "Service Revenue" {
		t.Fatalf("expected revenue details, got %+v", body.Data.Revenue)
	}
}

func TestLedgerHandlerGetPnLErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "validation", err: domain.NewValidationError("from must be before to"), statusCode: nethttp.StatusBadRequest, code: "BAD_REQUEST"},
		{name: "not found", err: domain.ErrNotFound, statusCode: nethttp.StatusNotFound, code: "AGENT_NOT_FOUND"},
		{name: "persistence", err: errors.Join(domain.ErrPersistence, errors.New("query failed")), statusCode: nethttp.StatusInternalServerError, code: "FAILED_TO_FETCH_PNL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewLedgerHandler(&fakePnLUsecase{err: tt.err})
			req := httptest.NewRequest(nethttp.MethodGet, "/api/ledger/0xabc/pnl", nil)
			rec := httptest.NewRecorder()

			serveGetPnL(handler, rec, req)

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

func serveGetPnL(handler *LedgerHandler, rec *httptest.ResponseRecorder, req *nethttp.Request) {
	router := chi.NewRouter()
	router.Get("/api/ledger/{walletAddress}/pnl", handler.GetPnL)
	router.ServeHTTP(rec, req)
}
