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
	"io"
	"log"
	"math/big"
	nethttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/harundarat/rive/backend/pkg/apierror"
	"github.com/harundarat/rive/backend/pkg/response"
)

const (
	quickNodeWebhookMaxBodyBytes = 5 << 20
	quickNodeWebhookReplayWindow = 5 * time.Minute

	orderCreatedTopic  = "0x3c141f0d4da6a61756fc0700a74952cd02a98ab09cc68d83468ef0aade55759a"
	orderReleasedTopic = "0xd91f42113b172b52ce79098fcccffbdce809144244eae0a9aba15aa700714ee0"
	orderRefundedTopic = "0xe18debf292fbfc1f009a15d4e4932c1da2be34d7343f86f50989d09a04571e1b"
)

type escrowEventKind string

const (
	escrowEventOrderCreated  escrowEventKind = "order_created"
	escrowEventOrderReleased escrowEventKind = "order_released"
	escrowEventOrderRefunded escrowEventKind = "order_refunded"
)

type QuickNodeWebhookHandler struct {
	secret                string
	escrowContractAddress string
	workOrderEvents       domain.WorkOrderOnchainEventUsecase
	now                   func() time.Time
	logf                  func(format string, args ...any)
}

func NewQuickNodeWebhookHandler(
	secret string,
	escrowContractAddress string,
	workOrderEvents domain.WorkOrderOnchainEventUsecase,
) *QuickNodeWebhookHandler {
	return &QuickNodeWebhookHandler{
		secret:                secret,
		escrowContractAddress: strings.TrimSpace(escrowContractAddress),
		workOrderEvents:       workOrderEvents,
		now:                   time.Now,
		logf:                  log.Printf,
	}
}

func (h *QuickNodeWebhookHandler) HandleEscrowEvents(w nethttp.ResponseWriter, r *nethttp.Request) {
	if strings.TrimSpace(h.secret) == "" {
		response.Error(w, apierror.New(nethttp.StatusInternalServerError, "WEBHOOK_SECRET_NOT_CONFIGURED", "quicknode webhook secret is not configured"))
		return
	}
	if strings.TrimSpace(h.escrowContractAddress) == "" {
		response.Error(w, apierror.New(nethttp.StatusInternalServerError, "ESCROW_CONTRACT_ADDRESS_NOT_CONFIGURED", "escrow contract address is not configured"))
		return
	}
	if !common.IsHexAddress(h.escrowContractAddress) {
		response.Error(w, apierror.New(nethttp.StatusInternalServerError, "INVALID_ESCROW_CONTRACT_ADDRESS", "escrow contract address is invalid"))
		return
	}
	if h.workOrderEvents == nil {
		response.Error(w, apierror.New(nethttp.StatusInternalServerError, "WORK_ORDER_EVENTS_NOT_CONFIGURED", "work order event usecase is not configured"))
		return
	}

	body, err := readQuickNodeWebhookBody(w, r)
	if err != nil {
		response.Error(w, apierror.New(nethttp.StatusBadRequest, "BAD_REQUEST", err.Error()))
		return
	}

	nonce := r.Header.Get("X-QN-Nonce")
	timestamp := r.Header.Get("X-QN-Timestamp")
	signature := r.Header.Get("X-QN-Signature")
	if nonce == "" || timestamp == "" || signature == "" {
		response.Error(w, apierror.New(nethttp.StatusBadRequest, "BAD_REQUEST", "missing quicknode signature headers"))
		return
	}
	if err := h.validateTimestamp(timestamp); err != nil {
		response.Error(w, apierror.New(nethttp.StatusBadRequest, "BAD_REQUEST", err.Error()))
		return
	}
	if !verifyQuickNodeSignature(h.secret, string(body), nonce, timestamp, signature) {
		response.Error(w, apierror.New(nethttp.StatusUnauthorized, "INVALID_SIGNATURE", "invalid quicknode webhook signature"))
		return
	}

	var payload quickNodeWebhookPayload
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&payload); err != nil {
		response.Error(w, apierror.New(nethttp.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.Error(w, apierror.New(nethttp.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}

	processed := 0
	for _, receipt := range payload.MatchingReceipts {
		for _, receiptLog := range receipt.Logs {
			event, ok, err := decodeEscrowEventLog(receiptLog, h.escrowContractAddress)
			if err != nil {
				response.Error(w, apierror.New(nethttp.StatusBadRequest, "BAD_REQUEST", err.Error()))
				return
			}
			if !ok {
				continue
			}

			processed++
			h.logEscrowEvent(event)

			updated, err := h.updateWorkOrderFromEscrowEvent(r.Context(), event)
			if err != nil {
				response.Error(w, apierror.New(nethttp.StatusInternalServerError, "FAILED_TO_UPDATE_WORK_ORDER", err.Error()))
				return
			}
			if !updated {
				h.logNoMatchingWorkOrder(event)
			}
		}
	}

	response.Success(w, nethttp.StatusOK, quickNodeWebhookResponse{
		EventsProcessed: processed,
	})
}

func (h *QuickNodeWebhookHandler) logEscrowEvent(event escrowEvent) {
	logMessage := fmt.Sprintf("quicknode %s event", event.Kind)
	if event.Removed {
		logMessage = fmt.Sprintf("quicknode %s reorg removal", event.Kind)
	}

	switch event.Kind {
	case escrowEventOrderCreated:
		h.logf(
			"%s tx_hash=%s log_index=%s block_number=%s removed=%t order_id=%s payer=%s payee=%s amount=%s spec_hash=%s",
			logMessage,
			event.TransactionHash,
			event.LogIndex,
			event.BlockNumber,
			event.Removed,
			event.OrderID.String(),
			event.Payer,
			event.Payee,
			event.Amount.String(),
			event.SpecHash,
		)
	case escrowEventOrderReleased:
		h.logf(
			"%s tx_hash=%s log_index=%s block_number=%s removed=%t order_id=%s payee=%s amount=%s",
			logMessage,
			event.TransactionHash,
			event.LogIndex,
			event.BlockNumber,
			event.Removed,
			event.OrderID.String(),
			event.Payee,
			event.Amount.String(),
		)
	case escrowEventOrderRefunded:
		h.logf(
			"%s tx_hash=%s log_index=%s block_number=%s removed=%t order_id=%s payer=%s amount=%s",
			logMessage,
			event.TransactionHash,
			event.LogIndex,
			event.BlockNumber,
			event.Removed,
			event.OrderID.String(),
			event.Payer,
			event.Amount.String(),
		)
	}
}

func (h *QuickNodeWebhookHandler) logNoMatchingWorkOrder(event escrowEvent) {
	switch event.Kind {
	case escrowEventOrderCreated:
		h.logf(
			"quicknode order_created no matching work_order tx_hash=%s log_index=%s removed=%t order_id=%s amount=%s spec_hash=%s",
			event.TransactionHash,
			event.LogIndex,
			event.Removed,
			event.OrderID.String(),
			event.Amount.String(),
			event.SpecHash,
		)
	case escrowEventOrderReleased:
		h.logf(
			"quicknode order_released no matching work_order tx_hash=%s log_index=%s removed=%t order_id=%s payee=%s amount=%s",
			event.TransactionHash,
			event.LogIndex,
			event.Removed,
			event.OrderID.String(),
			event.Payee,
			event.Amount.String(),
		)
	case escrowEventOrderRefunded:
		h.logf(
			"quicknode order_refunded no matching work_order tx_hash=%s log_index=%s removed=%t order_id=%s payer=%s amount=%s",
			event.TransactionHash,
			event.LogIndex,
			event.Removed,
			event.OrderID.String(),
			event.Payer,
			event.Amount.String(),
		)
	}
}

func (h *QuickNodeWebhookHandler) updateWorkOrderFromEscrowEvent(ctx context.Context, event escrowEvent) (bool, error) {
	now := h.now().UTC()
	switch event.Kind {
	case escrowEventOrderCreated:
		if event.Removed {
			return h.workOrderEvents.RollbackOrderCreated(ctx, domain.OrderCreatedWorkOrderRollback{
				SpecHash:        event.SpecHash,
				Payer:           event.Payer,
				Payee:           event.Payee,
				Amount:          copyBigInt(event.Amount),
				OnchainOrderID:  copyBigInt(event.OrderID),
				TransactionHash: event.TransactionHash,
				RolledBackAt:    now,
			})
		}

		return h.workOrderEvents.RecordOrderCreated(ctx, domain.OrderCreatedWorkOrderUpdate{
			SpecHash:        event.SpecHash,
			Payer:           event.Payer,
			Payee:           event.Payee,
			Amount:          copyBigInt(event.Amount),
			OnchainOrderID:  copyBigInt(event.OrderID),
			TransactionHash: event.TransactionHash,
			RecordedAt:      now,
		})
	case escrowEventOrderReleased:
		if event.Removed {
			return h.workOrderEvents.RollbackOrderReleased(ctx, domain.OrderReleasedWorkOrderRollback{
				Payee:          event.Payee,
				Amount:         copyBigInt(event.Amount),
				OnchainOrderID: copyBigInt(event.OrderID),
				RolledBackAt:   now,
			})
		}

		return h.workOrderEvents.RecordOrderReleased(ctx, domain.OrderReleasedWorkOrderUpdate{
			Payee:          event.Payee,
			Amount:         copyBigInt(event.Amount),
			OnchainOrderID: copyBigInt(event.OrderID),
			RecordedAt:     now,
		})
	case escrowEventOrderRefunded:
		if event.Removed {
			return h.workOrderEvents.RollbackOrderRefunded(ctx, domain.OrderRefundedWorkOrderRollback{
				Payer:          event.Payer,
				Amount:         copyBigInt(event.Amount),
				OnchainOrderID: copyBigInt(event.OrderID),
				RolledBackAt:   now,
			})
		}

		return h.workOrderEvents.RecordOrderRefunded(ctx, domain.OrderRefundedWorkOrderUpdate{
			Payer:          event.Payer,
			Amount:         copyBigInt(event.Amount),
			OnchainOrderID: copyBigInt(event.OrderID),
			RecordedAt:     now,
		})
	default:
		return false, fmt.Errorf("unsupported escrow event kind: %s", event.Kind)
	}
}

func readQuickNodeWebhookBody(w nethttp.ResponseWriter, r *nethttp.Request) ([]byte, error) {
	body, err := io.ReadAll(nethttp.MaxBytesReader(w, r.Body, quickNodeWebhookMaxBodyBytes))
	if err != nil {
		return nil, errors.New("failed to read request body")
	}
	defer r.Body.Close()

	if !strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		return body, nil
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("failed to decompress gzip payload")
	}
	defer gzipReader.Close()

	decodedBody, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, errors.New("failed to decompress gzip payload")
	}

	return decodedBody, nil
}

func (h *QuickNodeWebhookHandler) validateTimestamp(value string) error {
	unixSeconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return errors.New("invalid quicknode webhook timestamp")
	}

	signedAt := time.Unix(unixSeconds, 0)
	now := h.now()
	if now.Sub(signedAt) > quickNodeWebhookReplayWindow || signedAt.Sub(now) > quickNodeWebhookReplayWindow {
		return errors.New("quicknode webhook timestamp is outside the allowed replay window")
	}

	return nil
}

func verifyQuickNodeSignature(secret, payload, nonce, timestamp, givenSignature string) bool {
	signatureBytes, err := hex.DecodeString(trimHexPrefix(givenSignature))
	if err != nil || len(signatureBytes) != sha256.Size {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nonce + timestamp + payload))
	expectedSignature := mac.Sum(nil)

	return hmac.Equal(expectedSignature, signatureBytes)
}

func decodeEscrowEventLog(receiptLog quickNodeLog, escrowContractAddress string) (escrowEvent, bool, error) {
	if !strings.EqualFold(receiptLog.Address, escrowContractAddress) {
		return escrowEvent{}, false, nil
	}
	if len(receiptLog.Topics) == 0 {
		return escrowEvent{}, false, nil
	}

	topic := receiptLog.Topics[0]
	switch {
	case strings.EqualFold(topic, orderCreatedTopic):
		event, err := decodeOrderCreatedEventLog(receiptLog)
		return event, err == nil, err
	case strings.EqualFold(topic, orderReleasedTopic):
		event, err := decodeOrderReleasedEventLog(receiptLog)
		return event, err == nil, err
	case strings.EqualFold(topic, orderRefundedTopic):
		event, err := decodeOrderRefundedEventLog(receiptLog)
		return event, err == nil, err
	default:
		return escrowEvent{}, false, nil
	}
}

func decodeOrderCreatedEventLog(receiptLog quickNodeLog) (escrowEvent, error) {
	if len(receiptLog.Topics) != 4 {
		return escrowEvent{}, fmt.Errorf("invalid OrderCreated topics length: %d", len(receiptLog.Topics))
	}

	orderID, err := parseUint256Topic(receiptLog.Topics[1])
	if err != nil {
		return escrowEvent{}, fmt.Errorf("invalid OrderCreated orderID topic: %w", err)
	}
	payer, err := parseAddressTopic(receiptLog.Topics[2])
	if err != nil {
		return escrowEvent{}, fmt.Errorf("invalid OrderCreated payer topic: %w", err)
	}
	payee, err := parseAddressTopic(receiptLog.Topics[3])
	if err != nil {
		return escrowEvent{}, fmt.Errorf("invalid OrderCreated payee topic: %w", err)
	}

	data := trimHexPrefix(receiptLog.Data)
	if len(data) != 128 {
		return escrowEvent{}, fmt.Errorf("invalid OrderCreated data length: %d", len(data))
	}
	decodedData, err := hex.DecodeString(data)
	if err != nil {
		return escrowEvent{}, errors.New("invalid OrderCreated data")
	}
	amount := new(big.Int).SetBytes(decodedData[:32])
	specHash := common.BytesToHash(decodedData[32:64]).Hex()

	return escrowEvent{
		Kind:            escrowEventOrderCreated,
		TransactionHash: receiptLog.TransactionHash,
		LogIndex:        receiptLog.LogIndex,
		BlockNumber:     receiptLog.BlockNumber,
		Removed:         receiptLog.Removed,
		OrderID:         orderID,
		Payer:           payer.Hex(),
		Payee:           payee.Hex(),
		Amount:          amount,
		SpecHash:        specHash,
	}, nil
}

func decodeOrderReleasedEventLog(receiptLog quickNodeLog) (escrowEvent, error) {
	if len(receiptLog.Topics) != 3 {
		return escrowEvent{}, fmt.Errorf("invalid OrderReleased topics length: %d", len(receiptLog.Topics))
	}

	orderID, err := parseUint256Topic(receiptLog.Topics[1])
	if err != nil {
		return escrowEvent{}, fmt.Errorf("invalid OrderReleased orderID topic: %w", err)
	}
	payee, err := parseAddressTopic(receiptLog.Topics[2])
	if err != nil {
		return escrowEvent{}, fmt.Errorf("invalid OrderReleased payee topic: %w", err)
	}
	amount, err := parseUint256Data(receiptLog.Data, "OrderReleased")
	if err != nil {
		return escrowEvent{}, err
	}

	return escrowEvent{
		Kind:            escrowEventOrderReleased,
		TransactionHash: receiptLog.TransactionHash,
		LogIndex:        receiptLog.LogIndex,
		BlockNumber:     receiptLog.BlockNumber,
		Removed:         receiptLog.Removed,
		OrderID:         orderID,
		Payee:           payee.Hex(),
		Amount:          amount,
	}, nil
}

func decodeOrderRefundedEventLog(receiptLog quickNodeLog) (escrowEvent, error) {
	if len(receiptLog.Topics) != 3 {
		return escrowEvent{}, fmt.Errorf("invalid OrderRefunded topics length: %d", len(receiptLog.Topics))
	}

	orderID, err := parseUint256Topic(receiptLog.Topics[1])
	if err != nil {
		return escrowEvent{}, fmt.Errorf("invalid OrderRefunded orderID topic: %w", err)
	}
	payer, err := parseAddressTopic(receiptLog.Topics[2])
	if err != nil {
		return escrowEvent{}, fmt.Errorf("invalid OrderRefunded payer topic: %w", err)
	}
	amount, err := parseUint256Data(receiptLog.Data, "OrderRefunded")
	if err != nil {
		return escrowEvent{}, err
	}

	return escrowEvent{
		Kind:            escrowEventOrderRefunded,
		TransactionHash: receiptLog.TransactionHash,
		LogIndex:        receiptLog.LogIndex,
		BlockNumber:     receiptLog.BlockNumber,
		Removed:         receiptLog.Removed,
		OrderID:         orderID,
		Payer:           payer.Hex(),
		Amount:          amount,
	}, nil
}

func copyBigInt(value *big.Int) big.Int {
	if value == nil {
		return big.Int{}
	}

	return *new(big.Int).Set(value)
}

func parseUint256Topic(topic string) (*big.Int, error) {
	value := trimHexPrefix(topic)
	if len(value) != 64 {
		return nil, fmt.Errorf("expected 32-byte topic, got %d hex chars", len(value))
	}

	word, err := hex.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid uint256 topic")
	}

	return new(big.Int).SetBytes(word), nil
}

func parseAddressTopic(topic string) (common.Address, error) {
	value := trimHexPrefix(topic)
	if len(value) != 64 {
		return common.Address{}, fmt.Errorf("expected 32-byte topic, got %d hex chars", len(value))
	}

	word, err := hex.DecodeString(value)
	if err != nil {
		return common.Address{}, errors.New("invalid address topic")
	}

	return common.BytesToAddress(word[12:32]), nil
}

func parseUint256Data(data string, eventName string) (*big.Int, error) {
	value := trimHexPrefix(data)
	if len(value) != 64 {
		return nil, fmt.Errorf("invalid %s data length: %d", eventName, len(value))
	}

	word, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s data", eventName)
	}

	return new(big.Int).SetBytes(word), nil
}

func trimHexPrefix(value string) string {
	if len(value) >= 2 && (value[0:2] == "0x" || value[0:2] == "0X") {
		return value[2:]
	}

	return value
}

type quickNodeWebhookPayload struct {
	MatchingReceipts []quickNodeReceipt `json:"matchingReceipts"`
}

type quickNodeReceipt struct {
	Logs []quickNodeLog `json:"logs"`
}

type quickNodeLog struct {
	Address         string   `json:"address"`
	BlockNumber     string   `json:"blockNumber"`
	Data            string   `json:"data"`
	LogIndex        string   `json:"logIndex"`
	Removed         bool     `json:"removed"`
	Topics          []string `json:"topics"`
	TransactionHash string   `json:"transactionHash"`
}

type escrowEvent struct {
	Kind            escrowEventKind
	TransactionHash string
	LogIndex        string
	BlockNumber     string
	Removed         bool
	OrderID         *big.Int
	Payer           string
	Payee           string
	Amount          *big.Int
	SpecHash        string
}

type quickNodeWebhookResponse struct {
	EventsProcessed int `json:"events_processed"`
}
