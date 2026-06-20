package usecase

import (
	"context"

	"github.com/harundarat/rive/backend/internal/domain"
)

type HealthUsecase struct {
	storage domain.Storage
}

func NewHealthUsecase(storage domain.Storage) *HealthUsecase {
	return &HealthUsecase{storage: storage}
}

func (uc *HealthUsecase) CheckUploadStorage() (*domain.StorageUploadOutput, error) {
	data := map[string]any{
		"success": true,
	}
	fileHashes, err := uc.storage.UploadJSON(context.Background(), data)
	if err != nil {
		return nil, err
	}

	return fileHashes, nil

}
