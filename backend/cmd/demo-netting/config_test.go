package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestLoadDemoConfigDerivesAgentAddresses(t *testing.T) {
	configPath := writeTempConfig(t, minimalConfigYAML(t))

	cfg, err := loadDemoConfig(configPath)
	if err != nil {
		t.Fatalf("loadDemoConfig returned error: %v", err)
	}

	if cfg.APIBaseURL != "http://localhost:8080/api" {
		t.Fatalf("unexpected api base url: %q", cfg.APIBaseURL)
	}
	if len(cfg.agentByName) != 2 {
		t.Fatalf("expected two agents, got %d", len(cfg.agentByName))
	}
	if cfg.agentByName["alpha"].Address.Hex() == cfg.agentByName["beta"].Address.Hex() {
		t.Fatal("expected distinct derived agent addresses")
	}
	if cfg.Database.dsn() == "" || !strings.Contains(cfg.Database.dsn(), "sslmode=disable") {
		t.Fatalf("unexpected database dsn: %q", cfg.Database.dsn())
	}
}

func TestComputeDemoSummaryForTwentyIntents(t *testing.T) {
	configPath := filepath.Join("..", "..", "..", "demo", "netting.example.yaml")
	cfg, err := loadExampleWithoutPrivateKeys(t, configPath)
	if err != nil {
		t.Fatalf("load example config: %v", err)
	}

	summary, err := computeDemoSummary(cfg)
	if err != nil {
		t.Fatalf("computeDemoSummary returned error: %v", err)
	}

	if len(cfg.Intents) != 20 {
		t.Fatalf("expected 20 intents, got %d", len(cfg.Intents))
	}
	if summary.GrossAmount.String() != "79000000000000000000" {
		t.Fatalf("expected gross amount 79 rUSD, got %s", summary.GrossAmount.String())
	}
	if summary.NetAmount.String() != "23000000000000000000" {
		t.Fatalf("expected net amount 23 rUSD, got %s", summary.NetAmount.String())
	}
}

func TestIdempotencyKeyIncludesRunAndSequence(t *testing.T) {
	got := idempotencyKey("20260502T120000-abcd1234", 3)
	want := "netting-demo-20260502T120000-abcd1234-04"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFormatTokenAmount(t *testing.T) {
	if got := formatTokenAmount(mustBigInt("23000000000000000000"), 18); got != "23" {
		t.Fatalf("expected 23, got %q", got)
	}
	if got := formatTokenAmount(mustBigInt("1234500000000000000"), 18); got != "1.2345" {
		t.Fatalf("expected 1.2345, got %q", got)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "netting.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	return path
}

func minimalConfigYAML(t *testing.T) string {
	t.Helper()
	alpha := testPrivateKey(t)
	beta := testPrivateKey(t)
	return `api_base_url: "http://localhost:8080/api/"
rpc_url: "https://evmrpc.0g.ai"
explorer_tx_base_url: "https://chainscan.0g.ai/tx/"
database:
  host: "localhost"
  port: 5432
  user: "rive"
  password: "rive"
  name: "rive"
  sslmode: "disable"
contracts:
  rusd: "0x1111111111111111111111111111111111111111"
  netting_settlement: "0x2222222222222222222222222222222222222222"
agents:
  - name: "alpha"
    private_key: "` + alpha + `"
  - name: "beta"
    private_key: "` + beta + `"
intents:
  - payer: "alpha"
    payee: "beta"
    amount: "1000000000000000000"
`
}

func loadExampleWithoutPrivateKeys(t *testing.T, path string) (*demoConfig, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	replacements := []string{
		"0xREPLACE_WITH_AGENT_SCOUT_PRIVATE_KEY",
		"0xREPLACE_WITH_AGENT_ANALYST_PRIVATE_KEY",
		"0xREPLACE_WITH_AGENT_DATA_PRIVATE_KEY",
		"0xREPLACE_WITH_AGENT_VERIFIER_PRIVATE_KEY",
		"0xREPLACE_WITH_AGENT_ROUTER_PRIVATE_KEY",
	}
	content := string(data)
	for _, placeholder := range replacements {
		content = strings.Replace(content, placeholder, testPrivateKey(t), 1)
	}

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

func commonBytesHex(data []byte) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i, b := range data {
		out[i*2] = alphabet[b>>4]
		out[i*2+1] = alphabet[b&0x0f]
	}
	return string(out)
}
