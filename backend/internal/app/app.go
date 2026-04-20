package app

import (
	"fmt"
	"log"
	"net/http"

	"github.com/harundarat/rive/backend/internal/config"
	deliveryhttp "github.com/harundarat/rive/backend/internal/delivery/http"
	"github.com/harundarat/rive/backend/internal/infrastructure/database"
	"github.com/harundarat/rive/backend/internal/infrastructure/storage"
	"github.com/harundarat/rive/backend/internal/usecase"
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

	// Infrastructure Layer
	pgDB, err := database.Open(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	_ = pgDB

	zgClient, err := storage.NewZGStorageClient(cfg.ZeroG)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize 0G Storage client: %w", err)
	}
	_ = zgClient

	// Repository Layer

	// Usecase Layer
	healthUsecase := usecase.NewHealthUsecase(zgClient)

	// Handler Layer
	healthHandler := deliveryhttp.NewHealthHandler(healthUsecase)

	//Router
	router := deliveryhttp.NewRouter(healthHandler)

	log.Println("Starting application...")

	return &App{Config: cfg, Router: router}, nil
}
