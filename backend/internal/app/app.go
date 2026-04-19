package app

import (
	"fmt"
	"log"
	"net/http"

	"github.com/harundarat/rive/backend/internal/config"
	deliveryhttp "github.com/harundarat/rive/backend/internal/delivery/http"
	"github.com/harundarat/rive/backend/internal/infrastructure/database"
)

type App struct {
	Config *config.Config
	Router http.Handler
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

	// Repository Layer

	// Usecase Layer

	// Handler Layer
	healthHandler := deliveryhttp.NewHealthHandler()

	//Router
	router := deliveryhttp.NewRouter(healthHandler)

	log.Println("Starting application...")

	return &App{Config: cfg, Router: router}, nil
}
