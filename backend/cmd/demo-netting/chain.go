package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const erc20DemoABI = `[
	{"inputs":[{"internalType":"address","name":"to","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"}],"name":"mint","outputs":[],"stateMutability":"nonpayable","type":"function"},
	{"inputs":[{"internalType":"address","name":"spender","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"}],"name":"approve","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"},
	{"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
	{"inputs":[{"internalType":"address","name":"owner","type":"address"},{"internalType":"address","name":"spender","type":"address"}],"name":"allowance","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

type chainClient struct {
	client     *ethclient.Client
	chainID    *big.Int
	token      *bind.BoundContract
	settlement common.Address
}

func newChainClient(ctx context.Context, cfg *demoConfig) (*chainClient, error) {
	client, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("connect rpc: %w", err)
	}

	chainID, err := client.NetworkID(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("fetch chain id: %w", err)
	}

	parsed, err := abi.JSON(strings.NewReader(erc20DemoABI))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("parse erc20 abi: %w", err)
	}

	token := bind.NewBoundContract(common.HexToAddress(cfg.Contracts.RUSD), parsed, client, client, client)
	return &chainClient{
		client:     client,
		chainID:    chainID,
		token:      token,
		settlement: common.HexToAddress(cfg.Contracts.NettingSettlement),
	}, nil
}

func (c *chainClient) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
	}
}

func (c *chainClient) ensureAgentReady(ctx context.Context, agent *agentRuntime, requiredAllowance *big.Int, mintBuffer *big.Int, minNativeBalance *big.Int, decimals int) error {
	nativeBalance, err := c.client.BalanceAt(ctx, agent.Address, nil)
	if err != nil {
		return fmt.Errorf("fetch native balance for %s: %w", agent.Config.Name, err)
	}
	if nativeBalance.Cmp(minNativeBalance) < 0 {
		return fmt.Errorf("%s native balance too low: have %s wei, need at least %s wei", agent.Config.Name, nativeBalance.String(), minNativeBalance.String())
	}

	tokenBalance, err := c.tokenBalance(ctx, agent.Address)
	if err != nil {
		return err
	}
	if tokenBalance.Cmp(requiredAllowance) < 0 {
		mintAmount := new(big.Int).Sub(requiredAllowance, tokenBalance)
		mintAmount.Add(mintAmount, mintBuffer)
		if mintAmount.Sign() > 0 {
			fmt.Printf("Minting %s rUSD to %s (%s)\n", formatTokenAmount(mintAmount, decimals), agent.Config.Name, agent.Address.Hex())
			txHash, err := c.transact(ctx, agent.PrivateKey, "mint", agent.Address, mintAmount)
			if err != nil {
				return fmt.Errorf("mint rUSD for %s: %w", agent.Config.Name, err)
			}
			fmt.Printf("  mint tx: %s\n", txHash)
		}
	}

	if requiredAllowance.Sign() == 0 {
		return nil
	}

	allowance, err := c.allowance(ctx, agent.Address)
	if err != nil {
		return err
	}
	if allowance.Cmp(requiredAllowance) >= 0 {
		return nil
	}

	fmt.Printf("Approving %s rUSD from %s to NettingSettlement\n", formatTokenAmount(requiredAllowance, decimals), agent.Config.Name)
	txHash, err := c.transact(ctx, agent.PrivateKey, "approve", c.settlement, requiredAllowance)
	if err != nil {
		return fmt.Errorf("approve rUSD for %s: %w", agent.Config.Name, err)
	}
	fmt.Printf("  approve tx: %s\n", txHash)

	return nil
}

func (c *chainClient) tokenBalance(ctx context.Context, owner common.Address) (*big.Int, error) {
	var output []any
	if err := c.token.Call(&bind.CallOpts{Context: ctx}, &output, "balanceOf", owner); err != nil {
		return nil, fmt.Errorf("call balanceOf: %w", err)
	}
	return abiUint256Output(output, "balanceOf")
}

func (c *chainClient) allowance(ctx context.Context, owner common.Address) (*big.Int, error) {
	var output []any
	if err := c.token.Call(&bind.CallOpts{Context: ctx}, &output, "allowance", owner, c.settlement); err != nil {
		return nil, fmt.Errorf("call allowance: %w", err)
	}
	return abiUint256Output(output, "allowance")
}

func (c *chainClient) transact(ctx context.Context, privateKeyHex string, method string, args ...any) (string, error) {
	privateKey, err := crypto.HexToECDSA(trimHexPrefix(privateKeyHex))
	if err != nil {
		return "", err
	}
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, c.chainID)
	if err != nil {
		return "", err
	}
	auth.Context = ctx

	tx, err := c.token.Transact(auth, method, args...)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, c.client, tx)
	if err != nil {
		return "", err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return "", fmt.Errorf("%s tx reverted: %s", method, tx.Hash().Hex())
	}

	return tx.Hash().Hex(), nil
}

func abiUint256Output(output []any, method string) (*big.Int, error) {
	if len(output) != 1 {
		return nil, fmt.Errorf("%s returned %d outputs", method, len(output))
	}
	value, ok := output[0].(*big.Int)
	if !ok || value == nil {
		return nil, errors.New("uint256 output has unexpected type")
	}

	return new(big.Int).Set(value), nil
}
