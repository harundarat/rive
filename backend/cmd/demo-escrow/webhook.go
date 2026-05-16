package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

type quickNodeHeaders struct {
	Nonce     string
	Timestamp string
	Signature string
}

type quickNodeWebhookPayload struct {
	MatchingReceipts []quickNodeReceipt `json:"matchingReceipts"`
}

type quickNodeReceipt struct {
	BlockNumber string         `json:"blockNumber"`
	Logs        []quickNodeLog `json:"logs"`
}

type quickNodeLog struct {
	Address         string   `json:"address"`
	BlockNumber     string   `json:"blockNumber"`
	Data            string   `json:"data"`
	LogIndex        string   `json:"logIndex"`
	Removed         bool     `json:"removed"`
	Topics          []string `json:"topics"`
	TransactionHash string   `json:"transactionHash"`
}

func payloadFromReceipt(receipt *types.Receipt) ([]byte, error) {
	if receipt == nil || receipt.BlockNumber == nil {
		return nil, fmt.Errorf("receipt is missing block number")
	}

	logs := make([]quickNodeLog, 0, len(receipt.Logs))
	for _, receiptLog := range receipt.Logs {
		topics := make([]string, 0, len(receiptLog.Topics))
		for _, topic := range receiptLog.Topics {
			topics = append(topics, topic.Hex())
		}

		logs = append(logs, quickNodeLog{
			Address:         receiptLog.Address.Hex(),
			BlockNumber:     uint64Hex(receiptLog.BlockNumber),
			Data:            bytesHex(receiptLog.Data),
			LogIndex:        uint64Hex(uint64(receiptLog.Index)),
			Removed:         receiptLog.Removed,
			Topics:          topics,
			TransactionHash: receiptLog.TxHash.Hex(),
		})
	}

	payload := quickNodeWebhookPayload{
		MatchingReceipts: []quickNodeReceipt{
			{
				BlockNumber: uint64Hex(receipt.BlockNumber.Uint64()),
				Logs:        logs,
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal webhook payload: %w", err)
	}

	return data, nil
}

func signedQuickNodeHeaders(secret string, payload []byte) (quickNodeHeaders, error) {
	nonce, err := randomNonce()
	if err != nil {
		return quickNodeHeaders{}, err
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	return quickNodeHeaders{
		Nonce:     nonce,
		Timestamp: timestamp,
		Signature: quickNodeSignature(secret, payload, nonce, timestamp),
	}, nil
}

func quickNodeSignature(secret string, payload []byte, nonce string, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nonce + timestamp))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func randomNonce() (string, error) {
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", fmt.Errorf("generate quicknode nonce: %w", err)
	}

	return hex.EncodeToString(randomBytes[:]), nil
}

func bytesHex(data []byte) string {
	if len(data) == 0 {
		return "0x"
	}

	return "0x" + hex.EncodeToString(data)
}

func uint64Hex(value uint64) string {
	return fmt.Sprintf("0x%x", value)
}
