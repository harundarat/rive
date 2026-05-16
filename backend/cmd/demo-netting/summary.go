package main

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
)

type netPosition struct {
	Name    string
	Address string
	Amount  *big.Int
}

type demoSummary struct {
	GrossAmount *big.Int
	NetAmount   *big.Int
	Positions   []netPosition
}

func computeDemoSummary(cfg *demoConfig) (*demoSummary, error) {
	gross := big.NewInt(0)
	positions := map[string]*big.Int{}
	for name := range cfg.agentByName {
		positions[name] = big.NewInt(0)
	}

	for i, intent := range cfg.Intents {
		amount, err := parsePositiveBigInt(fmt.Sprintf("intents[%d].amount", i), intent.Amount)
		if err != nil {
			return nil, err
		}
		payer := strings.TrimSpace(intent.Payer)
		payee := strings.TrimSpace(intent.Payee)
		gross.Add(gross, amount)
		positions[payer].Sub(positions[payer], amount)
		positions[payee].Add(positions[payee], amount)
	}

	result := make([]netPosition, 0, len(positions))
	net := big.NewInt(0)
	for name, amount := range positions {
		if amount.Sign() < 0 {
			net.Add(net, new(big.Int).Abs(amount))
		}
		result = append(result, netPosition{
			Name:    name,
			Address: cfg.agentByName[name].Address.Hex(),
			Amount:  new(big.Int).Set(amount),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return &demoSummary{GrossAmount: gross, NetAmount: net, Positions: result}, nil
}

func outgoingTotals(cfg *demoConfig) (map[string]*big.Int, error) {
	totals := map[string]*big.Int{}
	for name := range cfg.agentByName {
		totals[name] = big.NewInt(0)
	}

	for i, intent := range cfg.Intents {
		amount, err := parsePositiveBigInt(fmt.Sprintf("intents[%d].amount", i), intent.Amount)
		if err != nil {
			return nil, err
		}
		totals[strings.TrimSpace(intent.Payer)].Add(totals[strings.TrimSpace(intent.Payer)], amount)
	}

	return totals, nil
}

func idempotencyKey(runID string, index int) string {
	return fmt.Sprintf("netting-demo-%s-%02d", runID, index+1)
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
