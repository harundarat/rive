package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	configPath := flag.String("config", "../demo/escrow.local.yaml", "path to escrow demo YAML config")
	flag.Parse()

	if err := run(context.Background(), *configPath); err != nil {
		fmt.Fprintf(os.Stderr, "demo-escrow failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string) error {
	cfg, err := loadDemoConfig(configPath)
	if err != nil {
		return err
	}

	runID, err := newRunID()
	if err != nil {
		return err
	}
	amount, err := parsePositiveBigInt("work_order.amount", cfg.WorkOrder.Amount)
	if err != nil {
		return err
	}
	minNative, err := parsePositiveOrZeroBigInt("minimum_native_balance_wei", cfg.MinimumNativeBalanceWei)
	if err != nil {
		return err
	}
	mintBuffer, err := parsePositiveOrZeroBigInt("mint_buffer_amount", cfg.MintBufferAmount)
	if err != nil {
		return err
	}
	timeout := time.Duration(cfg.PollTimeoutSeconds) * time.Second

	fmt.Println("Rive Escrow Demo")
	fmt.Printf("Run ID: %s\n", runID)
	fmt.Printf("Payer: %s (%s)\n", cfg.payerRuntime.Config.Name, cfg.payerRuntime.Address.Hex())
	fmt.Printf("Payee: %s (%s)\n", cfg.payeeRuntime.Config.Name, cfg.payeeRuntime.Address.Hex())
	fmt.Printf("Amount: %s rUSD\n\n", formatTokenAmount(amount, cfg.TokenDecimals))

	api := newAPIClient(cfg.APIBaseURL)
	fmt.Println("Checking backend health...")
	if err := api.checkHealth(ctx); err != nil {
		return err
	}

	db, err := openDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	fmt.Println("Seeding demo agents into Postgres...")
	if err := seedAgents(ctx, db, cfg); err != nil {
		return err
	}

	chain, err := newChainClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer chain.Close()

	fmt.Println("Checking wallet gas balances...")
	if err := chain.ensureNativeBalance(ctx, cfg.payerRuntime, minNative); err != nil {
		return err
	}
	if err := chain.ensureNativeBalance(ctx, cfg.payeeRuntime, minNative); err != nil {
		return err
	}

	fmt.Println("Preparing rUSD balance and escrow allowance...")
	if err := chain.ensurePayerTokenReady(ctx, cfg.payerRuntime, amount, mintBuffer, cfg.TokenDecimals); err != nil {
		return err
	}

	fmt.Println("\nCreating work order in backend...")
	workOrderRequest := cfg.workOrderRequest(runID)
	workOrder, err := api.createWorkOrder(ctx, workOrderRequest)
	if err != nil {
		return err
	}
	fmt.Printf("  work order id: %s\n", workOrder.ID)
	fmt.Printf("  spec hash: %s\n", workOrder.RootHash)
	fmt.Printf("  spec upload tx: %s\n", workOrder.TxHash)

	fmt.Println("\nCreating and funding escrow order on-chain...")
	orderID, createTxHash, createReceipt, err := chain.createOrder(ctx, cfg.payerRuntime, cfg.payeeRuntime.Address, amount, workOrder.RootHash)
	if err != nil {
		return err
	}
	onchainOrderID := orderID.String()
	fmt.Printf("  on-chain order id: %s\n", onchainOrderID)
	fmt.Printf("  createOrder tx: %s\n", createTxHash)

	fmt.Println("Relaying OrderCreated receipt to backend webhook...")
	if err := relayReceipt(ctx, api, cfg.QuickNodeWebhookSecret, createReceipt); err != nil {
		return err
	}
	if _, err := api.waitForWorkOrderStatus(ctx, onchainOrderID, "funded", timeout); err != nil {
		return err
	}
	fmt.Println("  backend status: funded")

	fmt.Println("\nUploading delivery payload to 0G Storage...")
	deliveryPayload := cfg.deliveryPayload(onchainOrderID)
	upload, err := api.uploadDelivery(ctx, deliveryPayload)
	if err != nil {
		return err
	}
	fmt.Printf("  delivery hash: %s\n", upload.RootHash)
	fmt.Printf("  delivery upload tx: %s\n", upload.TxHash)

	fmt.Println("Submitting signed delivery to backend...")
	signature, err := signDelivery(cfg.payeeRuntime.PrivateKey, onchainOrderID, upload.RootHash)
	if err != nil {
		return err
	}
	if err := api.submitDelivery(ctx, onchainOrderID, deliveryRequest{DeliveryHash: upload.RootHash, Signature: signature}); err != nil {
		return err
	}
	fmt.Println("  delivery accepted")

	fmt.Println("\nReleasing escrow order on-chain...")
	releaseTxHash, releaseReceipt, err := chain.releaseOrder(ctx, cfg.payerRuntime, orderID)
	if err != nil {
		return err
	}
	fmt.Printf("  releaseOrder tx: %s\n", releaseTxHash)

	fmt.Println("Relaying OrderReleased receipt to backend webhook...")
	if err := relayReceipt(ctx, api, cfg.QuickNodeWebhookSecret, releaseReceipt); err != nil {
		return err
	}
	finalStatus, err := api.waitForWorkOrderStatus(ctx, onchainOrderID, "completed", timeout)
	if err != nil {
		return err
	}
	fmt.Println("  backend status: completed")

	printSummary(cfg, runID, amount, workOrder, upload, finalStatus, onchainOrderID, createTxHash, releaseTxHash)
	return nil
}

func relayReceipt(ctx context.Context, api *apiClient, secret string, receipt *types.Receipt) error {
	payload, err := payloadFromReceipt(receipt)
	if err != nil {
		return err
	}
	headers, err := signedQuickNodeHeaders(secret, payload)
	if err != nil {
		return err
	}

	return api.relayWebhook(ctx, payload, headers)
}

func (c *demoConfig) workOrderRequest(runID string) workOrderRequest {
	criteria := make([]acceptanceCriteriaRequest, 0, len(c.WorkOrder.AcceptanceCriteria))
	for _, item := range c.WorkOrder.AcceptanceCriteria {
		criteria = append(criteria, acceptanceCriteriaRequest{
			ID:               item.ID,
			Description:      item.Description,
			VerificationHint: item.VerificationHint,
		})
	}

	return workOrderRequest{
		IdempotencyKey: "escrow-demo-" + runID,
		Parties: workOrderPartiesRequest{
			Payer: c.payerRuntime.Address.Hex(),
			Payee: c.payeeRuntime.Address.Hex(),
		},
		Task: workOrderTaskRequest{
			Title:       c.WorkOrder.Task.Title,
			Description: c.WorkOrder.Task.Description,
			Category:    c.WorkOrder.Task.Category,
		},
		Deliverable: workOrderDeliverableRequest{
			Format: c.WorkOrder.Deliverable.Format,
			Submission: workOrderSubmissionRequest{
				Method:   c.WorkOrder.Deliverable.Submission.Method,
				Endpoint: c.WorkOrder.Deliverable.Submission.Endpoint,
			},
		},
		AcceptanceCriteria: criteria,
		Compensation: workOrderCompensationRequest{
			Amount: c.WorkOrder.Amount,
			Asset:  c.WorkOrder.Asset,
			Chain:  c.WorkOrder.Chain,
		},
		Deadline: c.WorkOrder.Deadline,
	}
}

func (c *demoConfig) deliveryPayload(onchainOrderID string) map[string]any {
	payload := deepCopyMap(c.DeliveryPayload)
	if _, exists := payload["workOrderID"]; !exists {
		payload["workOrderID"] = onchainOrderID
	}

	return payload
}

func deepCopyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		if nested, ok := value.(map[string]any); ok {
			output[key] = deepCopyMap(nested)
			continue
		}
		if nestedList, ok := value.([]any); ok {
			output[key] = deepCopySlice(nestedList)
			continue
		}
		output[key] = value
	}

	return output
}

func deepCopySlice(input []any) []any {
	output := make([]any, len(input))
	for i, value := range input {
		if nested, ok := value.(map[string]any); ok {
			output[i] = deepCopyMap(nested)
			continue
		}
		if nestedList, ok := value.([]any); ok {
			output[i] = deepCopySlice(nestedList)
			continue
		}
		output[i] = value
	}

	return output
}

func signDelivery(privateKeyHex string, onchainOrderID string, deliveryHash string) (string, error) {
	privateKey, err := crypto.HexToECDSA(trimHexPrefix(privateKeyHex))
	if err != nil {
		return "", err
	}
	message := fmt.Sprintf("deliver:%s:%s", onchainOrderID, deliveryHash)
	signature, err := crypto.Sign(accounts.TextHash([]byte(message)), privateKey)
	if err != nil {
		return "", fmt.Errorf("sign delivery message: %w", err)
	}

	return "0x" + hex.EncodeToString(signature), nil
}

func printSummary(
	cfg *demoConfig,
	runID string,
	amount *big.Int,
	workOrder *workOrderResponse,
	delivery *storageUploadResponse,
	finalStatus *workOrderStatusResponse,
	onchainOrderID string,
	createTxHash string,
	releaseTxHash string,
) {
	fmt.Println("\n=== Rive Escrow Demo Summary ===")
	fmt.Printf("Run ID: %s\n", runID)
	fmt.Printf("Work order ID: %s\n", workOrder.ID)
	fmt.Printf("On-chain order ID: %s\n", onchainOrderID)
	fmt.Printf("Payer: %s (%s)\n", cfg.payerRuntime.Config.Name, cfg.payerRuntime.Address.Hex())
	fmt.Printf("Payee: %s (%s)\n", cfg.payeeRuntime.Config.Name, cfg.payeeRuntime.Address.Hex())
	fmt.Printf("Amount: %s rUSD\n", formatTokenAmount(amount, cfg.TokenDecimals))
	fmt.Printf("Spec hash: %s\n", workOrder.RootHash)
	fmt.Printf("Delivery hash: %s\n", delivery.RootHash)
	fmt.Printf("Final backend status: %s\n", finalStatus.Status)
	fmt.Printf("Create order tx: %s\n", createTxHash)
	if strings.TrimSpace(cfg.ExplorerTxBaseURL) != "" {
		fmt.Printf("Create explorer: %s%s\n", strings.TrimRight(cfg.ExplorerTxBaseURL, "/")+"/", createTxHash)
	}
	fmt.Printf("Release order tx: %s\n", releaseTxHash)
	if strings.TrimSpace(cfg.ExplorerTxBaseURL) != "" {
		fmt.Printf("Release explorer: %s%s\n", strings.TrimRight(cfg.ExplorerTxBaseURL, "/")+"/", releaseTxHash)
	}
	fmt.Println("\nHeadline: Work order funded, delivered, and released through escrow.")
}

func newRunID() (string, error) {
	var randomBytes [4]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}

	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(randomBytes[:]), nil
}

func formatTokenAmount(value *big.Int, decimals int) string {
	if value == nil {
		return "0"
	}
	sign := ""
	amount := new(big.Int).Set(value)
	if amount.Sign() < 0 {
		sign = "-"
		amount.Abs(amount)
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Div(amount, scale)
	fraction := new(big.Int).Mod(amount, scale)
	if fraction.Sign() == 0 {
		return sign + whole.String()
	}

	fractionText := fmt.Sprintf("%0*s", decimals, fraction.String())
	fractionText = strings.TrimRight(fractionText, "0")
	if len(fractionText) > 6 {
		fractionText = fractionText[:6]
	}

	return sign + whole.String() + "." + fractionText
}
