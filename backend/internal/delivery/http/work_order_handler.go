package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
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
	var request domain.WorkOrderSpecRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}

	output, err := h.workOrderUsecase.UploadSpec(r.Context(), request)
	if err != nil {
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", validationErr.Error()))
			return
		}
		if errors.Is(err, domain.ErrPersistence) {
			response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_CREATE_WORK_ORDER", err.Error()))
			return
		}
		if errors.Is(err, domain.ErrStorage) {
			response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_UPLOAD_TO_STORAGE", err.Error()))
			return
		}

		response.Error(w, apierror.New(http.StatusInternalServerError, "INTERNAL_ERROR", err.Error()))
		return
	}

	response.Success(w, http.StatusCreated, output)
}

func (h *WorkOrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	output, err := h.workOrderUsecase.GetByOnchainOrderID(r.Context(), chi.URLParam(r, "onchainOrderID"))
	if err != nil {
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", validationErr.Error()))
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, apierror.New(http.StatusNotFound, "WORK_ORDER_NOT_FOUND", "work order not found"))
			return
		}
		if errors.Is(err, domain.ErrPersistence) {
			response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_FETCH_WORK_ORDER", err.Error()))
			return
		}

		response.Error(w, apierror.New(http.StatusInternalServerError, "INTERNAL_ERROR", err.Error()))
		return
	}

	response.Success(w, http.StatusOK, output)
}

func (h *WorkOrderHandler) SubmitDelivery(w http.ResponseWriter, r *http.Request) {
	var request domain.WorkOrderDeliveryRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}

	output, err := h.workOrderUsecase.SubmitDelivery(r.Context(), chi.URLParam(r, "onchainOrderID"), request)
	if err != nil {
		h.writeDeliveryError(w, err)
		return
	}

	response.Success(w, http.StatusCreated, output)
}

func (h *WorkOrderHandler) writeDeliveryError(w http.ResponseWriter, err error) {
	var validationErr *domain.ValidationError
	if errors.As(err, &validationErr) {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", validationErr.Error()))
		return
	}
	if errors.Is(err, domain.ErrInvalidSignature) {
		response.Error(w, apierror.New(http.StatusUnauthorized, "INVALID_SIGNATURE", "invalid signature"))
		return
	}
	if errors.Is(err, domain.ErrPayeeMismatch) {
		response.Error(w, apierror.New(http.StatusForbidden, "PAYEE_MISMATCH", "signature does not match work order payee"))
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		response.Error(w, apierror.New(http.StatusNotFound, "WORK_ORDER_NOT_FOUND", "work order not found"))
		return
	}
	if errors.Is(err, domain.ErrOrderNotFunded) {
		response.Error(w, apierror.New(http.StatusConflict, "ORDER_NOT_FUNDED", "work order is not funded"))
		return
	}
	if errors.Is(err, domain.ErrDeliveryAlreadyPosted) {
		response.Error(w, apierror.New(http.StatusConflict, "DELIVERY_ALREADY_POSTED", "delivery already posted"))
		return
	}
	if errors.Is(err, domain.ErrPersistence) {
		response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_SUBMIT_DELIVERY", err.Error()))
		return
	}

	response.Error(w, apierror.New(http.StatusInternalServerError, "INTERNAL_ERROR", err.Error()))
}
