package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/harundarat/rive/backend/pkg/apierror"
	"github.com/harundarat/rive/backend/pkg/response"
)

type LedgerHandler struct {
	pnlUsecase domain.PnLUsecase
}

func NewLedgerHandler(pnlUsecase domain.PnLUsecase) *LedgerHandler {
	return &LedgerHandler{pnlUsecase: pnlUsecase}
}

func (h *LedgerHandler) GetPnL(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	output, err := h.pnlUsecase.GetPnL(
		r.Context(),
		chi.URLParam(r, "walletAddress"),
		query.Get("from"),
		query.Get("to"),
	)
	if err != nil {
		h.writePnLError(w, err)
		return
	}

	response.Success(w, http.StatusOK, output)
}

func (h *LedgerHandler) writePnLError(w http.ResponseWriter, err error) {
	var validationErr *domain.ValidationError
	if errors.As(err, &validationErr) {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", validationErr.Error()))
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		response.Error(w, apierror.New(http.StatusNotFound, "AGENT_NOT_FOUND", "agent not found"))
		return
	}
	if errors.Is(err, domain.ErrPersistence) {
		response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_FETCH_PNL", err.Error()))
		return
	}

	response.Error(w, apierror.New(http.StatusInternalServerError, "INTERNAL_ERROR", err.Error()))
}
