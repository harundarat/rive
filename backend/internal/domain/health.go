package domain

type HealthUsecase interface {
	CheckUploadZGStorage() (*ZGUploadOutput, error)
}
