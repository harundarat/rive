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

type NettingHandler struct {
	nettingUsecase domain.NettingUsecase
}

func NewNettingHandler(nettingUsecase domain.NettingUsecase) *NettingHandler {
	return &NettingHandler{nettingUsecase: nettingUsecase}
}

func (h *NettingHandler) CreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	var request domain.PaymentIntentRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}

	output, err := h.nettingUsecase.SubmitIntent(r.Context(), request)
	if err != nil {
		h.writePaymentIntentError(w, err)
		return
	}

	response.Success(w, http.StatusCreated, output)
}

func (h *NettingHandler) writePaymentIntentError(w http.ResponseWriter, err error) {
	var validationErr *domain.ValidationError
	if errors.As(err, &validationErr) {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", validationErr.Error()))
		return
	}
	if errors.Is(err, domain.ErrUnsupportedAsset) {
		response.Error(w, apierror.New(http.StatusBadRequest, "UNSUPPORTED_ASSET", "asset must be rUSD"))
		return
	}
	if errors.Is(err, domain.ErrPersistence) {
		response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_CREATE_PAYMENT_INTENT", err.Error()))
		return
	}

	response.Error(w, apierror.New(http.StatusInternalServerError, "INTERNAL_ERROR", err.Error()))
}
