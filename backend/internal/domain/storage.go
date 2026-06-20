package domain

import "context"

type StorageUploadOutput struct {
	TxHash   string `json:"tx_hash"`
	RootHash string `json:"root_hash"`
}

type Storage interface {
	UploadJSON(ctx context.Context, data any) (*StorageUploadOutput, error)
	UploadBytes(ctx context.Context, data []byte) (*StorageUploadOutput, error)
}
