package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID              uuid.UUID `json:"id"`
	AgentID0G       *string   `json:"agent_id_0g"`
	WalletAddress   string    `json:"wallet_address"`
	ReputationScore *float64  `json:"reputation_score"`
	MetadataCID     *string   `json:"metadata_cid"`
	CreatedAt       time.Time `json:"created_at"`
}

type AgentOnboardRequest struct {
	WalletAddress string `json:"wallet_address"`
}

type AgentUsecase interface {
	Onboard(ctx context.Context, req AgentOnboardRequest) (*Agent, error)
}

type AgentRepository interface {
	FindByWalletAddress(ctx context.Context, walletAddress string) (*Agent, error)
	Create(ctx context.Context, agent Agent) error
}
