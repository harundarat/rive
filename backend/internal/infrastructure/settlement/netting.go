package settlement

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/harundarat/rive/backend/internal/config"
	"github.com/harundarat/rive/backend/internal/domain"
)

const nettingSettlementABI = `[
	{
		"inputs": [
			{"internalType": "bytes32", "name": "batchHash", "type": "bytes32"},
			{"internalType": "address[]", "name": "debtors", "type": "address[]"},
			{"internalType": "uint256[]", "name": "debitAmounts", "type": "uint256[]"},
			{"internalType": "address[]", "name": "creditors", "type": "address[]"},
			{"internalType": "uint256[]", "name": "creditAmounts", "type": "uint256[]"}
		],
		"name": "settleBatch",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}
]`

type NettingGateway struct {
	disabled bool
	client   *ethclient.Client
	contract *bind.BoundContract
	auth     *bind.TransactOpts
}

func NewNettingGateway(netting config.NettingConfig) (*NettingGateway, error) {
	if strings.TrimSpace(netting.EVMRPC) == "" ||
		strings.TrimSpace(netting.SettlementAddress) == "" ||
		strings.TrimSpace(netting.SettlerPrivateKey) == "" {
		return &NettingGateway{disabled: true}, nil
	}
	if !common.IsHexAddress(netting.SettlementAddress) {
		return nil, fmt.Errorf("NETTING_SETTLEMENT_ADDRESS must be a valid Ethereum address")
	}

	client, err := ethclient.Dial(netting.EVMRPC)
	if err != nil {
		return nil, fmt.Errorf("connect netting settlement rpc: %w", err)
	}

	privateKey, err := crypto.HexToECDSA(trimHexPrefix(netting.SettlerPrivateKey))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("parse NETTING_SETTLER_PRIVATE_KEY: %w", err)
	}

	netCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	chainID, err := client.NetworkID(netCtx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("fetch chain id: %w", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("create netting transactor: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(nettingSettlementABI))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("parse netting settlement abi: %w", err)
	}

	contract := bind.NewBoundContract(common.HexToAddress(netting.SettlementAddress), parsedABI, client, client, client)

	return &NettingGateway{client: client, contract: contract, auth: auth}, nil
}

func (g *NettingGateway) SettleBatch(ctx context.Context, instruction domain.NettingSettlementInstruction) (*domain.NettingSettlementReceipt, error) {
	if instruction.SkipOnchainTx {
		return &domain.NettingSettlementReceipt{}, nil
	}
	if g == nil || g.disabled {
		return nil, errors.New("netting settlement gateway is not configured")
	}

	debtors, debitAmounts, err := settlementParties(instruction.Debtors)
	if err != nil {
		return nil, fmt.Errorf("invalid debtors: %w", err)
	}
	creditors, creditAmounts, err := settlementParties(instruction.Creditors)
	if err != nil {
		return nil, fmt.Errorf("invalid creditors: %w", err)
	}
	if len(debtors) == 0 || len(creditors) == 0 {
		return nil, errors.New("non-zero settlement requires debtors and creditors")
	}

	auth := *g.auth
	auth.Context = ctx

	tx, err := g.contract.Transact(&auth, "settleBatch", common.HexToHash(instruction.BatchHash), debtors, debitAmounts, creditors, creditAmounts)
	if err != nil {
		return nil, fmt.Errorf("submit settleBatch tx: %w", err)
	}

	receipt, err := bind.WaitMined(ctx, g.client, tx)
	if err != nil {
		return nil, fmt.Errorf("wait for settleBatch tx: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("settleBatch tx reverted: %s", tx.Hash().Hex())
	}

	txHash := tx.Hash().Hex()
	return &domain.NettingSettlementReceipt{TransactionHash: &txHash}, nil
}

func (g *NettingGateway) Close() {
	if g != nil && g.client != nil {
		g.client.Close()
	}
}

func settlementParties(parties []domain.NettingPartyAmount) ([]common.Address, []*big.Int, error) {
	addresses := make([]common.Address, 0, len(parties))
	amounts := make([]*big.Int, 0, len(parties))
	for _, party := range parties {
		if !common.IsHexAddress(party.Wallet) {
			return nil, nil, fmt.Errorf("invalid wallet address %q", party.Wallet)
		}
		if party.Amount.Sign() <= 0 {
			return nil, nil, errors.New("amount must be positive")
		}

		addresses = append(addresses, common.HexToAddress(party.Wallet))
		amounts = append(amounts, new(big.Int).Set(&party.Amount))
	}

	return addresses, amounts, nil
}

func trimHexPrefix(value string) string {
	return strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
}
