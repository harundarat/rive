package http

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/rive/backend/internal/domain"
)

const (
	testQuickNodeSecret       = "test-secret"
	testQuickNodeNonce        = "nonce-123"
	testQuickNodeTimestamp    = "1777046400"
	testEscrowContractAddress = "0x9A335EeEBE025794f0E0917e5E1d8De4925B245c"
	testOrderCreatedTxHash    = "0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70"
	testOrderReleasedTxHash   = "0xc1027a6be1b1a8e577438c73293ea5e1a7b5a3de84ec747e48ab0a3072db9245"
	testOrderRefundedTxHash   = "0xe1027a6be1b1a8e577438c73293ea5e1a7b5a3de84ec747e48ab0a3072db9245"
	testOrderCreatedSpecHash  = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

var testQuickNodeNow = time.Unix(1777046400, 0)

type fakeWorkOrderOnchainEventUsecase struct {
	recordInput             *domain.OrderCreatedWorkOrderUpdate
	rollbackInput           *domain.OrderCreatedWorkOrderRollback
	recordReleasedInput     *domain.OrderReleasedWorkOrderUpdate
	rollbackReleasedInput   *domain.OrderReleasedWorkOrderRollback
	recordRefundedInput     *domain.OrderRefundedWorkOrderUpdate
	rollbackRefundedInput   *domain.OrderRefundedWorkOrderRollback
	recordUpdated           bool
	rollbackUpdated         bool
	recordReleasedUpdated   bool
	rollbackReleasedUpdated bool
	recordRefundedUpdated   bool
	rollbackRefundedUpdated bool
	err                     error
}

func (uc *fakeWorkOrderOnchainEventUsecase) RecordOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderUpdate) (bool, error) {
	copy := event
	uc.recordInput = &copy
	if uc.err != nil {
		return false, uc.err
	}

	return uc.recordUpdated, nil
}

func (uc *fakeWorkOrderOnchainEventUsecase) RollbackOrderCreated(ctx context.Context, event domain.OrderCreatedWorkOrderRollback) (bool, error) {
	copy := event
	uc.rollbackInput = &copy
	if uc.err != nil {
		return false, uc.err
	}

	return uc.rollbackUpdated, nil
}

func (uc *fakeWorkOrderOnchainEventUsecase) RecordOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderUpdate) (bool, error) {
	copy := event
	uc.recordReleasedInput = &copy
	if uc.err != nil {
		return false, uc.err
	}

	return uc.recordReleasedUpdated, nil
}

func (uc *fakeWorkOrderOnchainEventUsecase) RollbackOrderReleased(ctx context.Context, event domain.OrderReleasedWorkOrderRollback) (bool, error) {
	copy := event
	uc.rollbackReleasedInput = &copy
	if uc.err != nil {
		return false, uc.err
	}

	return uc.rollbackReleasedUpdated, nil
}

func (uc *fakeWorkOrderOnchainEventUsecase) RecordOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderUpdate) (bool, error) {
	copy := event
	uc.recordRefundedInput = &copy
	if uc.err != nil {
		return false, uc.err
	}

	return uc.recordRefundedUpdated, nil
}

func (uc *fakeWorkOrderOnchainEventUsecase) RollbackOrderRefunded(ctx context.Context, event domain.OrderRefundedWorkOrderRollback) (bool, error) {
	copy := event
	uc.rollbackRefundedInput = &copy
	if uc.err != nil {
		return false, uc.err
	}

	return uc.rollbackRefundedUpdated, nil
}

func TestQuickNodeWebhookHandlerOrderCreatedSuccess(t *testing.T) {
	handler, logs, workOrderEvents := newTestQuickNodeWebhookHandler()
	payload := validQuickNodeOrderCreatedPayload(false, testEscrowContractAddress, orderCreatedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, nil)
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(*logs) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(*logs))
	}
	logLine := (*logs)[0]
	for _, expected := range []string{
		"quicknode order_created event",
		"tx_hash=0xd31c0da964d78e3647319d57ab9ec84636e715fbb90751b61d46a3040b5d8c70",
		"log_index=0x3",
		"block_number=0x1db02ad",
		"removed=false",
		"order_id=1",
		"amount=1000000",
		"spec_hash=" + testOrderCreatedSpecHash,
	} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("expected log to contain %q, got %q", expected, logLine)
		}
	}
	if workOrderEvents.recordInput == nil {
		t.Fatal("expected order created event to be recorded")
	}
	if workOrderEvents.rollbackInput != nil {
		t.Fatal("expected no rollback for non-removed event")
	}
	if workOrderEvents.recordInput.SpecHash != testOrderCreatedSpecHash {
		t.Fatalf("expected spec hash %q, got %q", testOrderCreatedSpecHash, workOrderEvents.recordInput.SpecHash)
	}
	if !strings.EqualFold(workOrderEvents.recordInput.Payer, "0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73") {
		t.Fatalf("expected payer from event, got %q", workOrderEvents.recordInput.Payer)
	}
	if !strings.EqualFold(workOrderEvents.recordInput.Payee, "0x1111111111111111111111111111111111111111") {
		t.Fatalf("expected payee from event, got %q", workOrderEvents.recordInput.Payee)
	}
	if workOrderEvents.recordInput.Amount.String() != "1000000" {
		t.Fatalf("expected amount 1000000, got %s", workOrderEvents.recordInput.Amount.String())
	}
	if workOrderEvents.recordInput.OnchainOrderID.String() != "1" {
		t.Fatalf("expected order id 1, got %s", workOrderEvents.recordInput.OnchainOrderID.String())
	}
	if workOrderEvents.recordInput.TransactionHash != testOrderCreatedTxHash {
		t.Fatalf("expected tx hash %q, got %q", testOrderCreatedTxHash, workOrderEvents.recordInput.TransactionHash)
	}
	if !workOrderEvents.recordInput.RecordedAt.Equal(testQuickNodeNow.UTC()) {
		t.Fatalf("expected recorded_at %s, got %s", testQuickNodeNow.UTC(), workOrderEvents.recordInput.RecordedAt)
	}

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			EventsProcessed int `json:"events_processed"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success || body.Data.EventsProcessed != 1 {
		t.Fatalf("expected success with 1 processed event, got %+v", body)
	}
}

func TestQuickNodeWebhookHandlerOrderReleasedSuccess(t *testing.T) {
	handler, logs, workOrderEvents := newTestQuickNodeWebhookHandler()
	payload := validQuickNodeOrderReleasedPayload(false, testEscrowContractAddress, orderReleasedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, nil)
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(*logs) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(*logs))
	}
	logLine := (*logs)[0]
	for _, expected := range []string{
		"quicknode order_released event",
		"tx_hash=" + testOrderReleasedTxHash,
		"log_index=0x8",
		"block_number=0x1e45042",
		"removed=false",
		"order_id=1",
		"amount=1000000",
	} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("expected log to contain %q, got %q", expected, logLine)
		}
	}
	if !strings.Contains(strings.ToLower(logLine), "payee=0x933a54d5d7a6c0c9e6318395a74cb99ac1c56934") {
		t.Fatalf("expected log to contain release payee, got %q", logLine)
	}
	if workOrderEvents.recordReleasedInput == nil {
		t.Fatal("expected order released event to be recorded")
	}
	if workOrderEvents.rollbackReleasedInput != nil {
		t.Fatal("expected no rollback for non-removed event")
	}
	if !strings.EqualFold(workOrderEvents.recordReleasedInput.Payee, "0x933a54d5d7a6c0c9e6318395a74cb99ac1c56934") {
		t.Fatalf("expected payee from event, got %q", workOrderEvents.recordReleasedInput.Payee)
	}
	if workOrderEvents.recordReleasedInput.Amount.String() != "1000000" {
		t.Fatalf("expected amount 1000000, got %s", workOrderEvents.recordReleasedInput.Amount.String())
	}
	if workOrderEvents.recordReleasedInput.OnchainOrderID.String() != "1" {
		t.Fatalf("expected order id 1, got %s", workOrderEvents.recordReleasedInput.OnchainOrderID.String())
	}
	if !workOrderEvents.recordReleasedInput.RecordedAt.Equal(testQuickNodeNow.UTC()) {
		t.Fatalf("expected recorded_at %s, got %s", testQuickNodeNow.UTC(), workOrderEvents.recordReleasedInput.RecordedAt)
	}

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			EventsProcessed int `json:"events_processed"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success || body.Data.EventsProcessed != 1 {
		t.Fatalf("expected success with 1 processed event, got %+v", body)
	}
}

func TestQuickNodeWebhookHandlerOrderRefundedSuccess(t *testing.T) {
	handler, logs, workOrderEvents := newTestQuickNodeWebhookHandler()
	payload := validQuickNodeOrderRefundedPayload(false, testEscrowContractAddress, orderRefundedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, nil)
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(*logs) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(*logs))
	}
	logLine := (*logs)[0]
	for _, expected := range []string{
		"quicknode order_refunded event",
		"tx_hash=" + testOrderRefundedTxHash,
		"log_index=0x8",
		"block_number=0x1e45043",
		"removed=false",
		"order_id=1",
		"amount=1000000",
	} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("expected log to contain %q, got %q", expected, logLine)
		}
	}
	if !strings.Contains(strings.ToLower(logLine), "payer=0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73") {
		t.Fatalf("expected log to contain refund payer, got %q", logLine)
	}
	if workOrderEvents.recordRefundedInput == nil {
		t.Fatal("expected order refunded event to be recorded")
	}
	if workOrderEvents.rollbackRefundedInput != nil {
		t.Fatal("expected no rollback for non-removed event")
	}
	if !strings.EqualFold(workOrderEvents.recordRefundedInput.Payer, "0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73") {
		t.Fatalf("expected payer from event, got %q", workOrderEvents.recordRefundedInput.Payer)
	}
	if workOrderEvents.recordRefundedInput.Amount.String() != "1000000" {
		t.Fatalf("expected amount 1000000, got %s", workOrderEvents.recordRefundedInput.Amount.String())
	}
	if workOrderEvents.recordRefundedInput.OnchainOrderID.String() != "1" {
		t.Fatalf("expected order id 1, got %s", workOrderEvents.recordRefundedInput.OnchainOrderID.String())
	}
	if !workOrderEvents.recordRefundedInput.RecordedAt.Equal(testQuickNodeNow.UTC()) {
		t.Fatalf("expected recorded_at %s, got %s", testQuickNodeNow.UTC(), workOrderEvents.recordRefundedInput.RecordedAt)
	}
}

func TestQuickNodeWebhookHandlerInvalidSignature(t *testing.T) {
	handler, _, _ := newTestQuickNodeWebhookHandler()
	payload := validQuickNodeOrderCreatedPayload(false, testEscrowContractAddress, orderCreatedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, func(req *nethttp.Request) {
		req.Header.Set("X-QN-Signature", strings.Repeat("0", 64))
	})
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestQuickNodeWebhookHandlerNoMatchingWorkOrder(t *testing.T) {
	handler, logs, workOrderEvents := newTestQuickNodeWebhookHandler()
	workOrderEvents.recordUpdated = false
	payload := validQuickNodeOrderCreatedPayload(false, testEscrowContractAddress, orderCreatedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, nil)
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if workOrderEvents.recordInput == nil {
		t.Fatal("expected order created event to be recorded")
	}
	if len(*logs) != 2 {
		t.Fatalf("expected event and no-match logs, got %d", len(*logs))
	}
	if !strings.Contains((*logs)[1], "quicknode order_created no matching work_order") {
		t.Fatalf("expected no matching work order log, got %q", (*logs)[1])
	}
}

func TestQuickNodeWebhookHandlerPersistenceError(t *testing.T) {
	handler, _, workOrderEvents := newTestQuickNodeWebhookHandler()
	workOrderEvents.err = errors.New("database unavailable")
	payload := validQuickNodeOrderCreatedPayload(false, testEscrowContractAddress, orderCreatedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, nil)
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Error.Code != "FAILED_TO_UPDATE_WORK_ORDER" {
		t.Fatalf("expected update error code, got %q", body.Error.Code)
	}
}

func TestQuickNodeWebhookHandlerStorageError(t *testing.T) {
	handler, _, workOrderEvents := newTestQuickNodeWebhookHandler()
	workOrderEvents.err = fmt.Errorf("%w: upload escrow journal entry: unavailable", domain.ErrStorage)
	payload := validQuickNodeOrderCreatedPayload(false, testEscrowContractAddress, orderCreatedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, nil)
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Error.Code != "FAILED_TO_UPLOAD_TO_0G_STORAGE" {
		t.Fatalf("expected storage error code, got %q", body.Error.Code)
	}
}

func TestQuickNodeWebhookHandlerMissingSignatureHeaders(t *testing.T) {
	handler, _, _ := newTestQuickNodeWebhookHandler()
	req := httptest.NewRequest(nethttp.MethodPost, "/api/webhooks/quicknode/escrow-events", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestQuickNodeWebhookHandlerMalformedJSON(t *testing.T) {
	handler, _, _ := newTestQuickNodeWebhookHandler()
	req := signedQuickNodeWebhookRequest(t, "{", nil)
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestQuickNodeWebhookHandlerGzipPayload(t *testing.T) {
	handler, logs, workOrderEvents := newTestQuickNodeWebhookHandler()
	payload := validQuickNodeOrderCreatedPayload(false, testEscrowContractAddress, orderCreatedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, func(req *nethttp.Request) {
		req.Body = ioNopCloser(gzipPayload(t, payload))
		req.Header.Set("Content-Encoding", "gzip")
	})
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(*logs) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(*logs))
	}
	if workOrderEvents.recordInput == nil {
		t.Fatal("expected order created event to be recorded")
	}
}

func TestQuickNodeWebhookHandlerIgnoresUnrelatedLog(t *testing.T) {
	tests := []struct {
		name            string
		contractAddress string
		topic           string
	}{
		{
			name:            "unrelated contract address",
			contractAddress: "0x1111111111111111111111111111111111111111",
			topic:           orderCreatedTopic,
		},
		{
			name:            "unrelated topic",
			contractAddress: testEscrowContractAddress,
			topic:           "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, logs, workOrderEvents := newTestQuickNodeWebhookHandler()
			payload := validQuickNodeOrderCreatedPayload(false, tt.contractAddress, tt.topic)
			req := signedQuickNodeWebhookRequest(t, payload, nil)
			rec := httptest.NewRecorder()

			handler.HandleEscrowEvents(rec, req)

			if rec.Code != nethttp.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if len(*logs) != 0 {
				t.Fatalf("expected no logged event, got %d", len(*logs))
			}
			if workOrderEvents.recordInput != nil ||
				workOrderEvents.rollbackInput != nil ||
				workOrderEvents.recordReleasedInput != nil ||
				workOrderEvents.rollbackReleasedInput != nil ||
				workOrderEvents.recordRefundedInput != nil ||
				workOrderEvents.rollbackRefundedInput != nil {
				t.Fatalf("expected no work order update, got %+v", workOrderEvents)
			}

			var body struct {
				Data struct {
					EventsProcessed int `json:"events_processed"`
				} `json:"data"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if body.Data.EventsProcessed != 0 {
				t.Fatalf("expected 0 processed events, got %d", body.Data.EventsProcessed)
			}
		})
	}
}

func TestQuickNodeWebhookHandlerRemovedLog(t *testing.T) {
	handler, logs, workOrderEvents := newTestQuickNodeWebhookHandler()
	payload := validQuickNodeOrderCreatedPayload(true, testEscrowContractAddress, orderCreatedTopic)
	req := signedQuickNodeWebhookRequest(t, payload, nil)
	rec := httptest.NewRecorder()

	handler.HandleEscrowEvents(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(*logs) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(*logs))
	}
	if !strings.Contains((*logs)[0], "quicknode order_created reorg removal") || !strings.Contains((*logs)[0], "removed=true") {
		t.Fatalf("expected reorg removal log, got %q", (*logs)[0])
	}
	if workOrderEvents.rollbackInput == nil {
		t.Fatal("expected removed event to rollback order created")
	}
	if workOrderEvents.recordInput != nil {
		t.Fatal("expected no record call for removed event")
	}
	if workOrderEvents.rollbackInput.SpecHash != testOrderCreatedSpecHash {
		t.Fatalf("expected rollback spec hash %q, got %q", testOrderCreatedSpecHash, workOrderEvents.rollbackInput.SpecHash)
	}
	if !strings.EqualFold(workOrderEvents.rollbackInput.Payer, "0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73") {
		t.Fatalf("expected rollback payer from event, got %q", workOrderEvents.rollbackInput.Payer)
	}
	if !strings.EqualFold(workOrderEvents.rollbackInput.Payee, "0x1111111111111111111111111111111111111111") {
		t.Fatalf("expected rollback payee from event, got %q", workOrderEvents.rollbackInput.Payee)
	}
	if workOrderEvents.rollbackInput.Amount.String() != "1000000" {
		t.Fatalf("expected rollback amount 1000000, got %s", workOrderEvents.rollbackInput.Amount.String())
	}
	if workOrderEvents.rollbackInput.OnchainOrderID.String() != "1" {
		t.Fatalf("expected rollback order id 1, got %s", workOrderEvents.rollbackInput.OnchainOrderID.String())
	}
	if workOrderEvents.rollbackInput.TransactionHash != testOrderCreatedTxHash {
		t.Fatalf("expected rollback tx hash %q, got %q", testOrderCreatedTxHash, workOrderEvents.rollbackInput.TransactionHash)
	}
	if !workOrderEvents.rollbackInput.RolledBackAt.Equal(testQuickNodeNow.UTC()) {
		t.Fatalf("expected rollback time %s, got %s", testQuickNodeNow.UTC(), workOrderEvents.rollbackInput.RolledBackAt)
	}
}

func TestQuickNodeWebhookHandlerRemovedReleaseAndRefundLogs(t *testing.T) {
	tests := []struct {
		name    string
		payload func() string
		assert  func(t *testing.T, workOrderEvents *fakeWorkOrderOnchainEventUsecase, logs []string)
	}{
		{
			name: "released",
			payload: func() string {
				return validQuickNodeOrderReleasedPayload(true, testEscrowContractAddress, orderReleasedTopic)
			},
			assert: func(t *testing.T, workOrderEvents *fakeWorkOrderOnchainEventUsecase, logs []string) {
				t.Helper()

				if !strings.Contains(logs[0], "quicknode order_released reorg removal") || !strings.Contains(logs[0], "removed=true") {
					t.Fatalf("expected release reorg removal log, got %q", logs[0])
				}
				if workOrderEvents.rollbackReleasedInput == nil {
					t.Fatal("expected removed release event to rollback order released")
				}
				if workOrderEvents.recordReleasedInput != nil {
					t.Fatal("expected no record call for removed release event")
				}
				if !strings.EqualFold(workOrderEvents.rollbackReleasedInput.Payee, "0x933a54d5d7a6c0c9e6318395a74cb99ac1c56934") {
					t.Fatalf("expected rollback payee from event, got %q", workOrderEvents.rollbackReleasedInput.Payee)
				}
				if workOrderEvents.rollbackReleasedInput.Amount.String() != "1000000" {
					t.Fatalf("expected rollback amount 1000000, got %s", workOrderEvents.rollbackReleasedInput.Amount.String())
				}
				if workOrderEvents.rollbackReleasedInput.OnchainOrderID.String() != "1" {
					t.Fatalf("expected rollback order id 1, got %s", workOrderEvents.rollbackReleasedInput.OnchainOrderID.String())
				}
				if !workOrderEvents.rollbackReleasedInput.RolledBackAt.Equal(testQuickNodeNow.UTC()) {
					t.Fatalf("expected rollback time %s, got %s", testQuickNodeNow.UTC(), workOrderEvents.rollbackReleasedInput.RolledBackAt)
				}
			},
		},
		{
			name: "refunded",
			payload: func() string {
				return validQuickNodeOrderRefundedPayload(true, testEscrowContractAddress, orderRefundedTopic)
			},
			assert: func(t *testing.T, workOrderEvents *fakeWorkOrderOnchainEventUsecase, logs []string) {
				t.Helper()

				if !strings.Contains(logs[0], "quicknode order_refunded reorg removal") || !strings.Contains(logs[0], "removed=true") {
					t.Fatalf("expected refund reorg removal log, got %q", logs[0])
				}
				if workOrderEvents.rollbackRefundedInput == nil {
					t.Fatal("expected removed refund event to rollback order refunded")
				}
				if workOrderEvents.recordRefundedInput != nil {
					t.Fatal("expected no record call for removed refund event")
				}
				if !strings.EqualFold(workOrderEvents.rollbackRefundedInput.Payer, "0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73") {
					t.Fatalf("expected rollback payer from event, got %q", workOrderEvents.rollbackRefundedInput.Payer)
				}
				if workOrderEvents.rollbackRefundedInput.Amount.String() != "1000000" {
					t.Fatalf("expected rollback amount 1000000, got %s", workOrderEvents.rollbackRefundedInput.Amount.String())
				}
				if workOrderEvents.rollbackRefundedInput.OnchainOrderID.String() != "1" {
					t.Fatalf("expected rollback order id 1, got %s", workOrderEvents.rollbackRefundedInput.OnchainOrderID.String())
				}
				if !workOrderEvents.rollbackRefundedInput.RolledBackAt.Equal(testQuickNodeNow.UTC()) {
					t.Fatalf("expected rollback time %s, got %s", testQuickNodeNow.UTC(), workOrderEvents.rollbackRefundedInput.RolledBackAt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, logs, workOrderEvents := newTestQuickNodeWebhookHandler()
			req := signedQuickNodeWebhookRequest(t, tt.payload(), nil)
			rec := httptest.NewRecorder()

			handler.HandleEscrowEvents(rec, req)

			if rec.Code != nethttp.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if len(*logs) != 1 {
				t.Fatalf("expected 1 logged event, got %d", len(*logs))
			}
			tt.assert(t, workOrderEvents, *logs)
		})
	}
}

func TestQuickNodeWebhookHandlerMalformedEscrowEvent(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "invalid topics length",
			payload: quickNodePayloadWithEscrowLog(
				testEscrowContractAddress,
				"0x00000000000000000000000000000000000000000000000000000000000f4240",
				false,
				[]string{
					orderReleasedTopic,
					"0x0000000000000000000000000000000000000000000000000000000000000001",
				},
				testOrderReleasedTxHash,
				"0x8",
				"0x1e45042",
			),
		},
		{
			name: "invalid data length",
			payload: quickNodePayloadWithEscrowLog(
				testEscrowContractAddress,
				"0x1234",
				false,
				[]string{
					orderRefundedTopic,
					"0x0000000000000000000000000000000000000000000000000000000000000001",
					"0x00000000000000000000000026dea28e89dfdf4cd5ab9f63010bb46316ec3a73",
				},
				testOrderRefundedTxHash,
				"0x8",
				"0x1e45043",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, _ := newTestQuickNodeWebhookHandler()
			req := signedQuickNodeWebhookRequest(t, tt.payload, nil)
			rec := httptest.NewRecorder()

			handler.HandleEscrowEvents(rec, req)

			if rec.Code != nethttp.StatusBadRequest {
				t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func newTestQuickNodeWebhookHandler() (*QuickNodeWebhookHandler, *[]string, *fakeWorkOrderOnchainEventUsecase) {
	logs := []string{}
	workOrderEvents := &fakeWorkOrderOnchainEventUsecase{
		recordUpdated:           true,
		rollbackUpdated:         true,
		recordReleasedUpdated:   true,
		rollbackReleasedUpdated: true,
		recordRefundedUpdated:   true,
		rollbackRefundedUpdated: true,
	}
	handler := NewQuickNodeWebhookHandler(testQuickNodeSecret, testEscrowContractAddress, workOrderEvents)
	handler.now = func() time.Time { return testQuickNodeNow }
	handler.logf = func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	return handler, &logs, workOrderEvents
}

func signedQuickNodeWebhookRequest(t *testing.T, payload string, mutate func(req *nethttp.Request)) *nethttp.Request {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodPost, "/api/webhooks/quicknode/escrow-events", strings.NewReader(payload))
	req.Header.Set("X-QN-Nonce", testQuickNodeNonce)
	req.Header.Set("X-QN-Timestamp", testQuickNodeTimestamp)
	req.Header.Set("X-QN-Signature", signQuickNodeWebhookPayload(payload))
	if mutate != nil {
		mutate(req)
	}

	return req
}

func signQuickNodeWebhookPayload(payload string) string {
	mac := hmac.New(sha256.New, []byte(testQuickNodeSecret))
	mac.Write([]byte(testQuickNodeNonce + testQuickNodeTimestamp + payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func gzipPayload(t *testing.T, payload string) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write([]byte(payload)); err != nil {
		t.Fatalf("failed to write gzip payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close gzip payload: %v", err)
	}

	return &buf
}

func ioNopCloser(buf *bytes.Buffer) *readCloser {
	return &readCloser{Buffer: buf}
}

type readCloser struct {
	*bytes.Buffer
}

func (rc *readCloser) Close() error {
	return nil
}

func validQuickNodeOrderCreatedPayload(removed bool, contractAddress, topic string) string {
	return quickNodePayloadWithTransferAndEscrowLog(
		contractAddress,
		"0x00000000000000000000000000000000000000000000000000000000000f4240aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		removed,
		[]string{
			topic,
			"0x0000000000000000000000000000000000000000000000000000000000000001",
			"0x00000000000000000000000026dea28e89dfdf4cd5ab9f63010bb46316ec3a73",
			"0x0000000000000000000000001111111111111111111111111111111111111111",
		},
		testOrderCreatedTxHash,
		"0x3",
		"0x1db02ad",
		"0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73",
		contractAddress,
	)
}

func validQuickNodeOrderReleasedPayload(removed bool, contractAddress, topic string) string {
	return quickNodePayloadWithTransferAndEscrowLog(
		contractAddress,
		"0x00000000000000000000000000000000000000000000000000000000000f4240",
		removed,
		[]string{
			topic,
			"0x0000000000000000000000000000000000000000000000000000000000000001",
			"0x000000000000000000000000933a54d5d7a6c0c9e6318395a74cb99ac1c56934",
		},
		testOrderReleasedTxHash,
		"0x8",
		"0x1e45042",
		contractAddress,
		"0x933a54d5d7a6c0c9e6318395a74cb99ac1c56934",
	)
}

func validQuickNodeOrderRefundedPayload(removed bool, contractAddress, topic string) string {
	return quickNodePayloadWithTransferAndEscrowLog(
		contractAddress,
		"0x00000000000000000000000000000000000000000000000000000000000f4240",
		removed,
		[]string{
			topic,
			"0x0000000000000000000000000000000000000000000000000000000000000001",
			"0x00000000000000000000000026dea28e89dfdf4cd5ab9f63010bb46316ec3a73",
		},
		testOrderRefundedTxHash,
		"0x8",
		"0x1e45043",
		contractAddress,
		"0x26dea28e89dfdf4cd5ab9f63010bb46316ec3a73",
	)
}

func quickNodePayloadWithTransferAndEscrowLog(
	contractAddress string,
	data string,
	removed bool,
	topics []string,
	txHash string,
	logIndex string,
	blockNumber string,
	transferFrom string,
	transferTo string,
) string {
	escrowTopics, _ := json.Marshal(topics)
	transferTopics, _ := json.Marshal([]string{
		"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
		addressTopic(transferFrom),
		addressTopic(transferTo),
	})

	return fmt.Sprintf(`{
  "matchingReceipts": [
    {
      "blockNumber": "%s",
      "logs": [
        {
          "address": "0xb053e106d5236e4c4cd1b7da0ac51ba0b318c7a0",
          "blockNumber": "%s",
          "data": "0x00000000000000000000000000000000000000000000000000000000000f4240",
          "logIndex": "0x0",
          "removed": false,
          "topics": %s,
          "transactionHash": "%s"
        },
        {
          "address": "%s",
          "blockNumber": "%s",
          "data": "%s",
          "logIndex": "%s",
          "removed": %t,
          "topics": %s,
          "transactionHash": "%s"
        }
      ]
    }
  ]
}`, blockNumber, blockNumber, string(transferTopics), txHash, contractAddress, blockNumber, data, logIndex, removed, string(escrowTopics), txHash)
}

func quickNodePayloadWithEscrowLog(
	contractAddress string,
	data string,
	removed bool,
	topics []string,
	txHash string,
	logIndex string,
	blockNumber string,
) string {
	escrowTopics, _ := json.Marshal(topics)

	return fmt.Sprintf(`{
  "matchingReceipts": [
    {
      "blockNumber": "%s",
      "logs": [
        {
          "address": "%s",
          "blockNumber": "%s",
          "data": "%s",
          "logIndex": "%s",
          "removed": %t,
          "topics": %s,
          "transactionHash": "%s"
        }
      ]
    }
  ]
}`, blockNumber, contractAddress, blockNumber, data, logIndex, removed, string(escrowTopics), txHash)
}

func addressTopic(address string) string {
	return "0x000000000000000000000000" + strings.ToLower(trimHexPrefix(address))
}
