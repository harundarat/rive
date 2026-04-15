package app

import (
	"fmt"
	"log"

	"github.com/harundarat/rive/backend/internal/config"
)

type App struct {
	Config *config.Config
}

func Initialize() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	log.Println("Starting application...")

	return &App{Config: cfg}, nil
}
