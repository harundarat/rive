package domain

type HealthUsecase interface {
	CheckUploadStorage() (*StorageUploadOutput, error)
}
