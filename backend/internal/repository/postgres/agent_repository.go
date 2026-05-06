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
		WHERE LOWER(wallet_address) = LOWER($1)
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

func (r *AgentRepository) Create(ctx context.Context, agent domain.Agent) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO agents (id, wallet_address, created_at)
		VALUES ($1, $2, $3)
	`, agent.ID, agent.WalletAddress, agent.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}
