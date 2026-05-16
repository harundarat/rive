package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestLoadDemoConfigDerivesParties(t *testing.T) {
	configPath := writeTempConfig(t, minimalConfigYAML(t))

	cfg, err := loadDemoConfig(configPath)
	if err != nil {
		t.Fatalf("loadDemoConfig returned error: %v", err)
	}

	if cfg.APIBaseURL != "http://localhost:8080/api" {
		t.Fatalf("unexpected api base url: %q", cfg.APIBaseURL)
	}
	if cfg.payerRuntime.Address.Hex() == cfg.payeeRuntime.Address.Hex() {
		t.Fatal("expected distinct derived addresses")
	}
	if cfg.Database.dsn() == "" || !strings.Contains(cfg.Database.dsn(), "sslmode=disable") {
		t.Fatalf("unexpected database dsn: %q", cfg.Database.dsn())
	}
	if cfg.WorkOrder.Amount != "10000000000000000000" {
		t.Fatalf("unexpected amount: %q", cfg.WorkOrder.Amount)
	}
}

func TestExampleConfigShape(t *testing.T) {
	configPath := filepath.Join("..", "..", "..", "demo", "escrow.example.yaml")
	cfg, err := loadExampleWithoutPrivateKeys(t, configPath)
	if err != nil {
		t.Fatalf("load example config: %v", err)
	}

	if cfg.WorkOrder.Task.Title == "" {
		t.Fatal("expected example work order title")
	}
	if len(cfg.WorkOrder.AcceptanceCriteria) != 1 {
		t.Fatalf("expected one acceptance criterion, got %d", len(cfg.WorkOrder.AcceptanceCriteria))
	}
	if len(cfg.DeliveryPayload) == 0 {
		t.Fatal("expected example delivery payload")
	}
}

func TestLoadDemoConfigValidation(t *testing.T) {
	valid := minimalConfigYAML(t)
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing webhook secret",
			content: strings.Replace(valid, `quicknode_webhook_secret: "test-secret"`, `quicknode_webhook_secret: ""`, 1),
			want:    "quicknode_webhook_secret is required",
		},
		{
			name:    "invalid private key",
			content: strings.Replace(valid, `private_key: "0x`, `private_key: "not-a-key`, 1),
			want:    "payer.private_key is invalid",
		},
		{
			name:    "bad escrow address",
			content: strings.Replace(valid, `escrow: "0x2222222222222222222222222222222222222222"`, `escrow: "bad"`, 1),
			want:    "contracts.escrow must be a valid Ethereum address",
		},
		{
			name:    "non-positive amount",
			content: strings.Replace(valid, `amount: "10000000000000000000"`, `amount: "0"`, 1),
			want:    "work_order.amount must be greater than zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadDemoConfig(writeTempConfig(t, tt.content))
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error to contain %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestDeliveryPayloadAddsWorkOrderIDWithoutMutatingConfig(t *testing.T) {
	cfg, err := loadDemoConfig(writeTempConfig(t, minimalConfigYAML(t)))
	if err != nil {
		t.Fatalf("loadDemoConfig returned error: %v", err)
	}

	payload := cfg.deliveryPayload("0")
	if payload["workOrderID"] != "0" {
		t.Fatalf("expected workOrderID 0, got %v", payload["workOrderID"])
	}
	if _, exists := cfg.DeliveryPayload["workOrderID"]; exists {
		t.Fatal("expected source delivery payload to remain unchanged")
	}
}

func TestSignDeliveryRecoversPayeeAddress(t *testing.T) {
	cfg, err := loadDemoConfig(writeTempConfig(t, minimalConfigYAML(t)))
	if err != nil {
		t.Fatalf("loadDemoConfig returned error: %v", err)
	}

	signature, err := signDelivery(cfg.payeeRuntime.PrivateKey, "0", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("signDelivery returned error: %v", err)
	}
	recovered, err := recoverSignedAddress("deliver:0:0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", signature)
	if err != nil {
		t.Fatalf("recover signature: %v", err)
	}
	if !strings.EqualFold(recovered, cfg.payeeRuntime.Address.Hex()) {
		t.Fatalf("expected %s, got %s", cfg.payeeRuntime.Address.Hex(), recovered)
	}
}

func TestQuickNodeSignatureUsesExpectedHMAC(t *testing.T) {
	payload := []byte(`{"matchingReceipts":[]}`)
	nonce := "nonce-1"
	timestamp := "1770000000"
	got := quickNodeSignature("secret", payload, nonce, timestamp)

	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(nonce + timestamp))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "escrow.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	return path
}

func minimalConfigYAML(t *testing.T) string {
	t.Helper()
	payer := testPrivateKey(t)
	payee := testPrivateKey(t)
	return `api_base_url: "http://localhost:8080/api/"
rpc_url: "https://evmrpc.0g.ai"
explorer_tx_base_url: "https://chainscan.0g.ai/tx/"
quicknode_webhook_secret: "test-secret"
database:
  host: "localhost"
  port: 5432
  user: "rive"
  password: "rive"
  name: "rive"
  sslmode: "disable"
contracts:
  rusd: "0x1111111111111111111111111111111111111111"
  escrow: "0x2222222222222222222222222222222222222222"
payer:
  name: "requester"
  private_key: "` + payer + `"
payee:
  name: "worker"
  private_key: "` + payee + `"
work_order:
  amount: "10000000000000000000"
  asset: "rUSD"
  chain: "0g-mainnet"
  task:
    title: "Generate cleaned review dataset"
    description: "Payee creates a JSON dataset."
    category: "data-processing"
  deliverable:
    format: "json"
    submission:
      method: "rive-storage"
      endpoint: "/api/storage/upload"
  acceptance_criteria:
    - id: "ac1"
      description: "Delivery is valid JSON."
  deadline: "2026-12-31T23:59:59Z"
delivery_payload:
  result:
    status: "completed"
`
}

func loadExampleWithoutPrivateKeys(t *testing.T, path string) (*demoConfig, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	content = strings.Replace(content, "0xREPLACE_WITH_PAYER_PRIVATE_KEY", testPrivateKey(t), 1)
	content = strings.Replace(content, "0xREPLACE_WITH_PAYEE_PRIVATE_KEY", testPrivateKey(t), 1)

	return loadDemoConfig(writeTempConfig(t, content))
}

func testPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	return "0x" + strings.ToLower(commonBytesHex(crypto.FromECDSA(key)))
}

func recoverSignedAddress(message string, signature string) (string, error) {
	signatureBytes, err := hex.DecodeString(trimHexPrefix(signature))
	if err != nil {
		return "", err
	}
	publicKey, err := crypto.SigToPub(accounts.TextHash([]byte(message)), signatureBytes)
	if err != nil {
		return "", err
	}

	return crypto.PubkeyToAddress(*publicKey).Hex(), nil
}

func commonBytesHex(data []byte) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i, b := range data {
		out[i*2] = alphabet[b>>4]
		out[i*2+1] = alphabet[b&0x0f]
	}
	return string(out)
}
