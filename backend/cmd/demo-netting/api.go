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
	Success bool      `json:"success"`
	Error   *apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type paymentIntentRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	Payer          string `json:"payer"`
	Payee          string `json:"payee"`
	Amount         string `json:"amount"`
	Asset          string `json:"asset"`
}

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 20 * time.Second},
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

func (c *apiClient) submitIntent(ctx context.Context, request paymentIntentRequest) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/payments/intent", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("submit %s: %w", request.IdempotencyKey, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read payment intent response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("submit %s returned %d: %s", request.IdempotencyKey, resp.StatusCode, string(respBody))
	}

	var envelope apiEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("decode payment intent response: %w", err)
	}
	if !envelope.Success {
		if envelope.Error != nil {
			return fmt.Errorf("submit %s failed: %s: %s", request.IdempotencyKey, envelope.Error.Code, envelope.Error.Message)
		}
		return fmt.Errorf("submit %s failed without error details", request.IdempotencyKey)
	}

	return nil
}
