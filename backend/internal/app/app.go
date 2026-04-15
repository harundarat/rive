package app

import (
	"fmt"
	"log"

	"github.com/harundarat/rive/backend/internal/config"
	"github.com/harundarat/rive/backend/internal/infrastructure/database"
)

type App struct {
	Config *config.Config
}

func Initialize() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	pgDB, err := database.Open(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	_ = pgDB

	log.Println("Starting application...")

	return &App{Config: cfg}, nil
}
