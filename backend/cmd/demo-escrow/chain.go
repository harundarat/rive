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

const escrowDemoABI = `[
	{"inputs":[],"name":"nextOrderID","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
	{"inputs":[{"internalType":"address","name":"payee","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"},{"internalType":"bytes32","name":"specHash","type":"bytes32"}],"name":"createOrder","outputs":[{"internalType":"uint256","name":"orderID","type":"uint256"}],"stateMutability":"nonpayable","type":"function"},
	{"inputs":[{"internalType":"uint256","name":"orderID","type":"uint256"}],"name":"releaseOrder","outputs":[],"stateMutability":"nonpayable","type":"function"}
]`

var orderCreatedTopicHash = common.HexToHash("0x3c141f0d4da6a61756fc0700a74952cd02a98ab09cc68d83468ef0aade55759a")

type chainClient struct {
	client        *ethclient.Client
	chainID       *big.Int
	token         *bind.BoundContract
	escrow        *bind.BoundContract
	escrowAddress common.Address
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

	tokenABI, err := abi.JSON(strings.NewReader(erc20DemoABI))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("parse erc20 abi: %w", err)
	}
	escrowABI, err := abi.JSON(strings.NewReader(escrowDemoABI))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("parse escrow abi: %w", err)
	}

	escrowAddress := common.HexToAddress(cfg.Contracts.Escrow)
	return &chainClient{
		client:        client,
		chainID:       chainID,
		token:         bind.NewBoundContract(common.HexToAddress(cfg.Contracts.RUSD), tokenABI, client, client, client),
		escrow:        bind.NewBoundContract(escrowAddress, escrowABI, client, client, client),
		escrowAddress: escrowAddress,
	}, nil
}

func (c *chainClient) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
	}
}

func (c *chainClient) ensureNativeBalance(ctx context.Context, agent *agentRuntime, minNativeBalance *big.Int) error {
	nativeBalance, err := c.client.BalanceAt(ctx, agent.Address, nil)
	if err != nil {
		return fmt.Errorf("fetch native balance for %s: %w", agent.Config.Name, err)
	}
	if nativeBalance.Cmp(minNativeBalance) < 0 {
		return fmt.Errorf("%s native balance too low: have %s wei, need at least %s wei", agent.Config.Name, nativeBalance.String(), minNativeBalance.String())
	}

	return nil
}

func (c *chainClient) ensurePayerTokenReady(ctx context.Context, payer *agentRuntime, amount *big.Int, mintBuffer *big.Int, decimals int) error {
	tokenBalance, err := c.tokenBalance(ctx, payer.Address)
	if err != nil {
		return err
	}
	if tokenBalance.Cmp(amount) < 0 {
		mintAmount := new(big.Int).Sub(amount, tokenBalance)
		mintAmount.Add(mintAmount, mintBuffer)
		if mintAmount.Sign() > 0 {
			fmt.Printf("Minting %s rUSD to %s (%s)\n", formatTokenAmount(mintAmount, decimals), payer.Config.Name, payer.Address.Hex())
			txHash, err := c.transactToken(ctx, payer.PrivateKey, "mint", payer.Address, mintAmount)
			if err != nil {
				return fmt.Errorf("mint rUSD for %s: %w", payer.Config.Name, err)
			}
			fmt.Printf("  mint tx: %s\n", txHash)
		}
	}

	allowance, err := c.allowance(ctx, payer.Address)
	if err != nil {
		return err
	}
	if allowance.Cmp(amount) >= 0 {
		return nil
	}

	fmt.Printf("Approving %s rUSD from %s to Escrow\n", formatTokenAmount(amount, decimals), payer.Config.Name)
	txHash, err := c.transactToken(ctx, payer.PrivateKey, "approve", c.escrowAddress, amount)
	if err != nil {
		return fmt.Errorf("approve rUSD for %s: %w", payer.Config.Name, err)
	}
	fmt.Printf("  approve tx: %s\n", txHash)

	return nil
}

func (c *chainClient) nextOrderID(ctx context.Context) (*big.Int, error) {
	var output []any
	if err := c.escrow.Call(&bind.CallOpts{Context: ctx}, &output, "nextOrderID"); err != nil {
		return nil, fmt.Errorf("call nextOrderID: %w", err)
	}

	return abiUint256Output(output, "nextOrderID")
}

func (c *chainClient) createOrder(ctx context.Context, payer *agentRuntime, payee common.Address, amount *big.Int, specHash string) (*big.Int, string, *types.Receipt, error) {
	expectedOrderID, err := c.nextOrderID(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	txHash, receipt, err := c.transactEscrow(ctx, payer.PrivateKey, "createOrder", payee, amount, common.HexToHash(specHash))
	if err != nil {
		return nil, "", nil, fmt.Errorf("create escrow order: %w", err)
	}

	orderID, err := orderIDFromCreatedReceipt(receipt, c.escrowAddress)
	if err != nil {
		return nil, "", nil, err
	}
	if orderID.Cmp(expectedOrderID) != 0 {
		fmt.Printf("Warning: expected on-chain order ID %s, receipt emitted %s\n", expectedOrderID.String(), orderID.String())
	}

	return orderID, txHash, receipt, nil
}

func (c *chainClient) releaseOrder(ctx context.Context, payer *agentRuntime, orderID *big.Int) (string, *types.Receipt, error) {
	txHash, receipt, err := c.transactEscrow(ctx, payer.PrivateKey, "releaseOrder", orderID)
	if err != nil {
		return "", nil, fmt.Errorf("release escrow order: %w", err)
	}

	return txHash, receipt, nil
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
	if err := c.token.Call(&bind.CallOpts{Context: ctx}, &output, "allowance", owner, c.escrowAddress); err != nil {
		return nil, fmt.Errorf("call allowance: %w", err)
	}
	return abiUint256Output(output, "allowance")
}

func (c *chainClient) transactToken(ctx context.Context, privateKeyHex string, method string, args ...any) (string, error) {
	txHash, _, err := c.transact(ctx, c.token, privateKeyHex, method, args...)
	return txHash, err
}

func (c *chainClient) transactEscrow(ctx context.Context, privateKeyHex string, method string, args ...any) (string, *types.Receipt, error) {
	return c.transact(ctx, c.escrow, privateKeyHex, method, args...)
}

func (c *chainClient) transact(ctx context.Context, contract *bind.BoundContract, privateKeyHex string, method string, args ...any) (string, *types.Receipt, error) {
	privateKey, err := crypto.HexToECDSA(trimHexPrefix(privateKeyHex))
	if err != nil {
		return "", nil, err
	}
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, c.chainID)
	if err != nil {
		return "", nil, err
	}
	auth.Context = ctx

	tx, err := contract.Transact(auth, method, args...)
	if err != nil {
		return "", nil, err
	}
	receipt, err := bind.WaitMined(ctx, c.client, tx)
	if err != nil {
		return "", nil, err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return "", nil, fmt.Errorf("%s tx reverted: %s", method, tx.Hash().Hex())
	}

	return tx.Hash().Hex(), receipt, nil
}

func orderIDFromCreatedReceipt(receipt *types.Receipt, escrowAddress common.Address) (*big.Int, error) {
	for _, receiptLog := range receipt.Logs {
		if receiptLog.Address != escrowAddress || len(receiptLog.Topics) == 0 {
			continue
		}
		if receiptLog.Topics[0] == orderCreatedTopicHash {
			if len(receiptLog.Topics) < 2 {
				return nil, errors.New("OrderCreated log is missing orderID topic")
			}
			return new(big.Int).SetBytes(receiptLog.Topics[1].Bytes()), nil
		}
	}

	return nil, errors.New("OrderCreated log not found in createOrder receipt")
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
