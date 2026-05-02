package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type apiClient struct {
	baseURL string
	client  *http.Client
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *apiError       `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type workOrderRequest struct {
	IdempotencyKey     string                       `json:"idempotency_key"`
	Parties            workOrderPartiesRequest      `json:"parties"`
	Task               workOrderTaskRequest         `json:"task"`
	Deliverable        workOrderDeliverableRequest  `json:"deliverable"`
	AcceptanceCriteria []acceptanceCriteriaRequest  `json:"acceptanceCriteria"`
	Compensation       workOrderCompensationRequest `json:"compensation"`
	Deadline           string                       `json:"deadline"`
}

type workOrderPartiesRequest struct {
	Payer string `json:"payer"`
	Payee string `json:"payee"`
}

type workOrderTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type workOrderDeliverableRequest struct {
	Format     string                     `json:"format"`
	Submission workOrderSubmissionRequest `json:"submission"`
}

type workOrderSubmissionRequest struct {
	Method   string `json:"method"`
	Endpoint string `json:"endpoint"`
}

type acceptanceCriteriaRequest struct {
	ID               string  `json:"id"`
	Description      string  `json:"description"`
	VerificationHint *string `json:"verificationHint,omitempty"`
}

type workOrderCompensationRequest struct {
	Amount string `json:"amount"`
	Asset  string `json:"asset"`
	Chain  string `json:"chain"`
}

type workOrderResponse struct {
	ID       string `json:"id"`
	RootHash string `json:"root_hash"`
	TxHash   string `json:"tx_hash"`
}

type storageUploadResponse struct {
	TxHash   string `json:"tx_hash"`
	RootHash string `json:"root_hash"`
}

type deliveryRequest struct {
	DeliveryHash string `json:"deliveryHash"`
	Signature    string `json:"signature"`
}

type workOrderStatusResponse struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	SpecHash       string  `json:"spec_hash"`
	DeliverableCID *string `json:"deliverable_cid"`
}

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *apiClient) checkHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health/", nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("backend health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("backend health check returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *apiClient) createWorkOrder(ctx context.Context, request workOrderRequest) (*workOrderResponse, error) {
	var output workOrderResponse
	if err := c.postJSON(ctx, "/work-orders", request, http.StatusCreated, &output); err != nil {
		return nil, err
	}

	return &output, nil
}

func (c *apiClient) uploadDelivery(ctx context.Context, payload any) (*storageUploadResponse, error) {
	var output storageUploadResponse
	if err := c.postJSON(ctx, "/storage/upload", payload, http.StatusCreated, &output); err != nil {
		return nil, err
	}

	return &output, nil
}

func (c *apiClient) submitDelivery(ctx context.Context, onchainOrderID string, request deliveryRequest) error {
	return c.postJSON(ctx, "/work-orders/"+onchainOrderID+"/delivery", request, http.StatusCreated, nil)
}

func (c *apiClient) relayWebhook(ctx context.Context, payload []byte, headers quickNodeHeaders) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/webhooks/quicknode/escrow-events", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QN-Nonce", headers.Nonce)
	req.Header.Set("X-QN-Timestamp", headers.Timestamp)
	req.Header.Set("X-QN-Signature", headers.Signature)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("relay escrow webhook: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read webhook response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay escrow webhook returned %d: %s", resp.StatusCode, string(body))
	}
	if err := decodeEnvelope(body, nil); err != nil {
		return fmt.Errorf("decode webhook response: %w", err)
	}

	return nil
}

func (c *apiClient) waitForWorkOrderStatus(ctx context.Context, onchainOrderID string, expected string, timeout time.Duration) (*workOrderStatusResponse, error) {
	deadline := time.Now().Add(timeout)
	for {
		output, notFound, err := c.getWorkOrderStatus(ctx, onchainOrderID)
		if err != nil {
			return nil, err
		}
		if !notFound && output.Status == expected {
			return output, nil
		}
		if time.Now().After(deadline) {
			if notFound {
				return nil, fmt.Errorf("timed out waiting for work order %s to become %s: not found", onchainOrderID, expected)
			}
			return nil, fmt.Errorf("timed out waiting for work order %s to become %s: current status %s", onchainOrderID, expected, output.Status)
		}

		time.Sleep(2 * time.Second)
	}
}

func (c *apiClient) getWorkOrderStatus(ctx context.Context, onchainOrderID string) (*workOrderStatusResponse, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/work-orders/"+onchainOrderID, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("fetch work order %s: %w", onchainOrderID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("read work order response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return &workOrderStatusResponse{}, true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("fetch work order %s returned %d: %s", onchainOrderID, resp.StatusCode, string(body))
	}

	var output workOrderStatusResponse
	if err := decodeEnvelope(body, &output); err != nil {
		return nil, false, fmt.Errorf("decode work order response: %w", err)
	}

	return &output, false, nil
}

func (c *apiClient) postJSON(ctx context.Context, path string, payload any, expectedStatus int, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", path, err)
	}
	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("post %s returned %d: %s", path, resp.StatusCode, string(respBody))
	}
	if err := decodeEnvelope(respBody, output); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}

	return nil
}

func decodeEnvelope(body []byte, output any) error {
	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		if envelope.Error != nil {
			return fmt.Errorf("%s: %s", envelope.Error.Code, envelope.Error.Message)
		}
		return fmt.Errorf("request failed without error details")
	}
	if output == nil {
		return nil
	}
	if len(envelope.Data) == 0 {
		return fmt.Errorf("response data is empty")
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return err
	}

	return nil
}
