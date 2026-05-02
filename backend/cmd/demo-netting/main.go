package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"
)

func main() {
	configPath := flag.String("config", "../demo/netting.local.yaml", "path to netting demo YAML config")
	flag.Parse()

	if err := run(context.Background(), *configPath); err != nil {
		fmt.Fprintf(os.Stderr, "demo-netting failed: %v\n", err)
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
	summary, err := computeDemoSummary(cfg)
	if err != nil {
		return err
	}
	outgoing, err := outgoingTotals(cfg)
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

	fmt.Println("Rive Netting Demo")
	fmt.Printf("Run ID: %s\n", runID)
	fmt.Printf("Agents: %d\n", len(cfg.Agents))
	fmt.Printf("Payment intents: %d\n", len(cfg.Intents))
	fmt.Printf("Expected gross volume: %s rUSD\n", formatTokenAmount(summary.GrossAmount, cfg.TokenDecimals))
	fmt.Printf("Expected net amount: %s rUSD\n\n", formatTokenAmount(summary.NetAmount, cfg.TokenDecimals))

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

	fmt.Println("Checking for existing pending netting state...")
	if err := ensureNoPendingNetting(ctx, db); err != nil {
		return err
	}

	fmt.Println("Seeding demo agents into Postgres...")
	if err := seedAgents(ctx, db, cfg); err != nil {
		return err
	}

	chain, err := newChainClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer chain.Close()

	fmt.Println("Preparing rUSD balances and approvals...")
	agentNames := make([]string, 0, len(cfg.agentByName))
	for name := range cfg.agentByName {
		agentNames = append(agentNames, name)
	}
	sort.Strings(agentNames)
	for _, name := range agentNames {
		required := outgoing[name]
		if required == nil {
			required = big.NewInt(0)
		}
		if err := chain.ensureAgentReady(ctx, cfg.agentByName[name], required, mintBuffer, minNative, cfg.TokenDecimals); err != nil {
			return err
		}
	}

	fmt.Println("\nSubmitting payment intents...")
	for i, intent := range cfg.Intents {
		payer := cfg.agentByName[strings.TrimSpace(intent.Payer)]
		payee := cfg.agentByName[strings.TrimSpace(intent.Payee)]
		key := idempotencyKey(runID, i)
		request := paymentIntentRequest{
			IdempotencyKey: key,
			Payer:          payer.Address.Hex(),
			Payee:          payee.Address.Hex(),
			Amount:         strings.TrimSpace(intent.Amount),
			Asset:          "rUSD",
		}
		if err := api.submitIntent(ctx, request); err != nil {
			return err
		}
		fmt.Printf("  %02d. %s -> %s : %s rUSD\n", i+1, intent.Payer, intent.Payee, formatTokenAmount(mustBigInt(intent.Amount), cfg.TokenDecimals))
	}

	timeout := time.Duration(cfg.PollTimeoutSeconds) * time.Second
	fmt.Printf("\nWaiting up to %s for netting batch settlement...\n", timeout)
	batch, err := pollBatchSettlement(ctx, db, runID, len(cfg.Intents), timeout)
	if err != nil {
		return err
	}

	printSummary(cfg, runID, summary, batch)
	return nil
}

func printSummary(cfg *demoConfig, runID string, summary *demoSummary, batch *batchResult) {
	fmt.Println("\n=== Rive Netting Demo Summary ===")
	fmt.Printf("Run ID: %s\n", runID)
	fmt.Printf("Payment intents: %d\n", len(cfg.Intents))
	fmt.Printf("Gross payment volume: %s rUSD\n", formatTokenAmount(summary.GrossAmount, cfg.TokenDecimals))
	fmt.Printf("Net settlement amount: %s rUSD\n", formatTokenAmount(summary.NetAmount, cfg.TokenDecimals))
	fmt.Printf("On-chain tx without Rive: %d\n", len(cfg.Intents))
	if batch.SettlementTxHash != nil {
		fmt.Println("On-chain tx with Rive: 1")
	} else {
		fmt.Println("On-chain tx with Rive: 0 (zero-net batch)")
	}
	fmt.Printf("Final net positions: %d\n", nonZeroPositionCount(summary.Positions))
	fmt.Printf("Settlement transfer count: %d\n", batch.SettlementTransferCount)
	fmt.Printf("Batch ID: %s\n", batch.ID)
	if batch.SettlementTxHash != nil {
		fmt.Printf("Settlement tx: %s\n", *batch.SettlementTxHash)
		if strings.TrimSpace(cfg.ExplorerTxBaseURL) != "" {
			fmt.Printf("Explorer: %s%s\n", strings.TrimRight(cfg.ExplorerTxBaseURL, "/")+"/", *batch.SettlementTxHash)
		}
	}

	fmt.Println("\nFinal net positions:")
	for _, position := range summary.Positions {
		if position.Amount.Sign() == 0 {
			continue
		}
		role := "receives"
		amount := new(big.Int).Set(position.Amount)
		if amount.Sign() < 0 {
			role = "pays"
			amount.Abs(amount)
		}
		fmt.Printf("  %-10s %8s %s rUSD (%s)\n", position.Name, role, formatTokenAmount(amount, cfg.TokenDecimals), position.Address)
	}

	fmt.Printf("\nHeadline: %d logical payments compressed into 1 on-chain settlement tx.\n", len(cfg.Intents))
}

func nonZeroPositionCount(positions []netPosition) int {
	count := 0
	for _, position := range positions {
		if position.Amount.Sign() != 0 {
			count++
		}
	}

	return count
}

func newRunID() (string, error) {
	var randomBytes [4]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}

	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(randomBytes[:]), nil
}

func mustBigInt(value string) *big.Int {
	amount, ok := new(big.Int).SetString(strings.TrimSpace(value), 10)
	if !ok {
		return big.NewInt(0)
	}

	return amount
}
