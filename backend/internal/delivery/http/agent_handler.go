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

type AgentHandler struct {
	agentUsecase domain.AgentUsecase
}

func NewAgentHandler(agentUsecase domain.AgentUsecase) *AgentHandler {
	return &AgentHandler{agentUsecase: agentUsecase}
}

func (h *AgentHandler) Onboard(w http.ResponseWriter, r *http.Request) {
	var req domain.AgentOnboardRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}

	agent, err := h.agentUsecase.Onboard(r.Context(), req)
	if err != nil {
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", validationErr.Error()))
			return
		}
		if errors.Is(err, domain.ErrPersistence) {
			response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_ONBOARD_AGENT", err.Error()))
			return
		}
		response.Error(w, apierror.New(http.StatusInternalServerError, "INTERNAL_ERROR", err.Error()))
		return
	}

	response.Success(w, http.StatusCreated, agent)
}
