package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentRepository struct {
	db *pgxpool.Pool
}

func NewAgentRepository(db *pgxpool.Pool) *AgentRepository {
	return &AgentRepository{db: db}
}

func (r *AgentRepository) FindByWalletAddress(ctx context.Context, walletAddress string) (*domain.Agent, error) {
	var agent domain.Agent
	err := r.db.QueryRow(ctx, `
		SELECT id, agent_id_0g, wallet_address, reputation_score, metadata_cid, created_at
		FROM agents
		WHERE wallet_address = $1
	`, walletAddress).Scan(
		&agent.ID,
		&agent.AgentID0G,
		&agent.WalletAddress,
		&agent.ReputationScore,
		&agent.MetadataCID,
		&agent.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find agent by wallet address: %w", err)
	}

	return &agent, nil
}
