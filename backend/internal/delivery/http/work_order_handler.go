package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/harundarat/rive/backend/pkg/apierror"
	"github.com/harundarat/rive/backend/pkg/response"
)

type WorkOrderHandler struct {
	workOrderUsecase domain.WorkOrderUsecase
}

func NewWorkOrderHandler(workOrderUsecase domain.WorkOrderUsecase) *WorkOrderHandler {
	return &WorkOrderHandler{workOrderUsecase: workOrderUsecase}
}

func (h *WorkOrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input domain.WorkOrderSpecInput
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&input); err != nil {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}

	output, err := h.workOrderUsecase.UploadSpec(r.Context(), input)
	if err != nil {
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", validationErr.Error()))
			return
		}

		response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_UPLOAD_TO_0G_STORAGE", err.Error()))
		return
	}

	response.Success(w, http.StatusCreated, output)
}
