package usecase

import (
	"context"

	"github.com/harundarat/rive/backend/internal/domain"
)

type HealthUsecase struct {
	zgStorage domain.ZGStorage
}

func NewHealthUsecase(zgStorage domain.ZGStorage) *HealthUsecase {
	return &HealthUsecase{zgStorage: zgStorage}
}

func (uc *HealthUsecase) CheckUploadZGStorage() (*domain.ZGUploadOutput, error) {
	data := map[string]any{
		"success": true,
	}
	fileHashes, err := uc.zgStorage.UploadJSON(context.Background(), data)
	if err != nil {
		return nil, err
	}

	return fileHashes, nil

}
