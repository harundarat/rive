package domain

import "context"

type ZGUploadOutput struct {
	TxHash   string
	RootHash string
}

type ZGStorage interface {
	UploadJSON(ctx context.Context, data map[string]any) (*ZGUploadOutput, error)
}
