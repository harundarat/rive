package database

import (
	"context"
	"fmt"
	"log"

	"github.com/harundarat/rive/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	dbpool, err := pgxpool.New(context.Background(), cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	log.Println("Database connected")

	return dbpool, err
}
