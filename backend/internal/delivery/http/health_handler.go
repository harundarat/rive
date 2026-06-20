package http

import (
	"net/http"

	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/harundarat/rive/backend/pkg/apierror"
	"github.com/harundarat/rive/backend/pkg/response"
)

type HealthHandler struct {
	HealthUsecase domain.HealthUsecase
}

func NewHealthHandler(healthUsecase domain.HealthUsecase) *HealthHandler {
	return &HealthHandler{
		HealthUsecase: healthUsecase,
	}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	response.Success(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) CheckUploadStorage(w http.ResponseWriter, r *http.Request) {
	fileHashes, err := h.HealthUsecase.CheckUploadStorage()
	if err != nil {
		response.Error(w, &apierror.APIError{
			HTTPStatus: http.StatusInternalServerError,
			Code:       "FAILED_TO_UPLOAD_TO_STORAGE",
			Message:    err.Error(),
		})
	}

	response.Success(w, http.StatusCreated, fileHashes)
}
