package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

type AgentUsecase struct {
	agentRepository domain.AgentRepository
	now             func() time.Time
	newID           func() (uuid.UUID, error)
}

func NewAgentUsecase(agentRepository domain.AgentRepository) *AgentUsecase {
	return &AgentUsecase{
		agentRepository: agentRepository,
		now:             time.Now,
		newID:           uuid.NewV7,
	}
}

func (uc *AgentUsecase) Onboard(ctx context.Context, req domain.AgentOnboardRequest) (*domain.Agent, error) {
	walletAddress := strings.TrimSpace(req.WalletAddress)
	if walletAddress == "" {
		return nil, domain.NewValidationError("wallet_address is required")
	}

	existing, err := uc.agentRepository.FindByWalletAddress(ctx, walletAddress)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%w: find existing agent: %w", domain.ErrPersistence, err)
	}

	id, err := uc.newID()
	if err != nil {
		return nil, fmt.Errorf("generate agent id: %w", err)
	}

	agent := domain.Agent{
		ID:            id,
		WalletAddress: walletAddress,
		CreatedAt:     uc.now().UTC(),
	}

	if err := uc.agentRepository.Create(ctx, agent); err != nil {
		existing, findErr := uc.agentRepository.FindByWalletAddress(ctx, walletAddress)
		if findErr == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("%w: create agent: %w", domain.ErrPersistence, err)
	}

	return &agent, nil
}
