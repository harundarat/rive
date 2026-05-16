package storage

import (
	"context"
	"errors"
	"os"

	zgcommon "github.com/0gfoundation/0g-storage-client/common"
	"github.com/0gfoundation/0g-storage-client/common/blockchain"
	"github.com/0gfoundation/0g-storage-client/core"
	"github.com/0gfoundation/0g-storage-client/indexer"
	"github.com/0gfoundation/0g-storage-client/transfer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/harundarat/rive/backend/internal/config"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/harundarat/rive/backend/pkg/canonicaljson"
	"github.com/openweb3/web3go"
	"github.com/sirupsen/logrus"
)

type ZGClient struct {
	W3      *web3go.Client
	Indexer *indexer.Client
}

func NewZGStorageClient(cfg config.ZeroGStorageConfig) (*ZGClient, error) {
	w3 := blockchain.MustNewWeb3(cfg.EVMRPC, cfg.PrivateKey)
	idx, err := indexer.NewClient(cfg.IndexerRPC, indexer.IndexerClientOption{
		LogOption: zgcommon.LogOption{
			LogLevel: logrus.InfoLevel,
		},
	})

	if err != nil {
		w3.Close()
		return nil, err
	}

	return &ZGClient{W3: w3, Indexer: idx}, nil
}

func (c *ZGClient) Close() {
	c.W3.Close()
}

func (c *ZGClient) UploadJSON(ctx context.Context, data any) (*domain.ZGUploadOutput, error) {
	jsonBytes, err := canonicalJSONBytes(data)
	if err != nil {
		return nil, err
	}

	return c.UploadBytes(ctx, jsonBytes)
}

func (c *ZGClient) UploadBytes(ctx context.Context, data []byte) (*domain.ZGUploadOutput, error) {
	tmpFile, err := os.CreateTemp("", "rive-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err = tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return nil, err
	}
	if err = tmpFile.Close(); err != nil {
		return nil, err
	}

	file, err := core.Open(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	defer file.Close()

	opt := transfer.UploadOption{
		Submitter:        common.Address{},
		Tags:             nil,
		FinalityRequired: transfer.TransactionPacked,
		TaskSize:         0,
		ExpectedReplica:  1,
		SkipTx:           false,
		FastMode:         false,
		Fee:              nil,
		Nonce:            nil,
		MaxGasPrice:      nil,
		NRetries:         0,
		Step:             0,
		Method:           "min",
		FullTrusted:      true,
		EncryptionKey:    nil,
	}
	fragmentSize := int64(4 * 1024 * 1024 * 1024)

	txHashes, roots, err := c.Indexer.SplitableUpload(ctx, c.W3, file, fragmentSize, opt)
	if err != nil {
		return nil, err
	}
	if len(txHashes) == 0 {
		return nil, errors.New("0g storage upload returned no transaction hashes")
	}
	if len(roots) == 0 {
		return nil, errors.New("0g storage upload returned no root hashes")
	}

	return &domain.ZGUploadOutput{TxHash: txHashes[0].String(), RootHash: roots[0].String()}, nil
}

func canonicalJSONBytes(data any) ([]byte, error) {
	return canonicaljson.Bytes(data)
}
