package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddlewareAllowsConfiguredOrigin(t *testing.T) {
	handler := corsMiddleware([]string{"http://localhost:3000"})(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusOK)
	}))
	req := httptest.NewRequest(nethttp.MethodGet, "/api/ledger/0xabc/pnl", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("expected Vary=Origin, got %q", got)
	}
}

func TestCORSMiddlewareHandlesPreflight(t *testing.T) {
	called := false
	handler := corsMiddleware([]string{"https://app.example.com"})(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		called = true
		w.WriteHeader(nethttp.StatusOK)
	}))
	req := httptest.NewRequest(nethttp.MethodOptions, "/api/ledger/0xabc/pnl", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", nethttp.MethodGet)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("expected preflight to return before calling next handler")
	}
	if rec.Code != nethttp.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != corsAllowedMethods {
		t.Fatalf("expected allowed methods %q, got %q", corsAllowedMethods, got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != corsAllowedHeaders {
		t.Fatalf("expected allowed headers %q, got %q", corsAllowedHeaders, got)
	}
}

func TestCORSMiddlewareRejectsUnconfiguredOrigin(t *testing.T) {
	handler := corsMiddleware([]string{"http://localhost:3000"})(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusOK)
	}))
	req := httptest.NewRequest(nethttp.MethodGet, "/api/ledger/0xabc/pnl", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allowed origin header, got %q", got)
	}
}
