package domain

import "context"

type ZGUploadOutput struct {
	TxHash   string `json:"tx_hash"`
	RootHash string `json:"root_hash"`
}

type ZGStorage interface {
	UploadJSON(ctx context.Context, data any) (*ZGUploadOutput, error)
}
