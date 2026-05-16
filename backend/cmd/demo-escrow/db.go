package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openDB(ctx context.Context, cfg *demoConfig) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.Database.dsn())
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

func seedAgents(ctx context.Context, db *pgxpool.Pool, cfg *demoConfig) error {
	if err := seedAgent(ctx, db, cfg.payerRuntime); err != nil {
		return err
	}
	if err := seedAgent(ctx, db, cfg.payeeRuntime); err != nil {
		return err
	}

	return nil
}

func seedAgent(ctx context.Context, db *pgxpool.Pool, agent *agentRuntime) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate agent id: %w", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO agents (
			id,
			agent_id_0g,
			wallet_address,
			reputation_score,
			metadata_cid,
			created_at
		) VALUES ($1, $2, $3, 0, $4, NOW())
		ON CONFLICT (wallet_address) DO UPDATE
		SET
			agent_id_0g = EXCLUDED.agent_id_0g,
			metadata_cid = EXCLUDED.metadata_cid
	`,
		id,
		agent.Config.AgentID0G,
		agent.Address.Hex(),
		agent.Config.MetadataCID,
	)
	if err != nil {
		return fmt.Errorf("upsert agent %s: %w", agent.Config.Name, err)
	}

	return nil
}
