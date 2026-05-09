package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/harundarat/rive/backend/internal/config"
	deliveryhttp "github.com/harundarat/rive/backend/internal/delivery/http"
	"github.com/harundarat/rive/backend/internal/infrastructure/database"
	"github.com/harundarat/rive/backend/internal/infrastructure/settlement"
	"github.com/harundarat/rive/backend/internal/infrastructure/storage"
	postgresrepo "github.com/harundarat/rive/backend/internal/repository/postgres"
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

	zgClient, err := storage.NewZGStorageClient(cfg.ZeroG)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize 0G Storage client: %w", err)
	}

	// Repository Layer
	agentRepository := postgresrepo.NewAgentRepository(pgDB)
	workOrderRepository := postgresrepo.NewWorkOrderRepository(pgDB, zgClient)
	pnlRepository := postgresrepo.NewPnLRepository(pgDB)
	nettingRepository := postgresrepo.NewNettingRepository(pgDB)

	nettingGateway, err := settlement.NewNettingGateway(cfg.ZeroG, cfg.Netting)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize netting settlement gateway: %w", err)
	}

	// Usecase Layer
	agentUsecase := usecase.NewAgentUsecase(agentRepository)
	healthUsecase := usecase.NewHealthUsecase(zgClient)
	workOrderUsecase := usecase.NewWorkOrderUsecase(zgClient, workOrderRepository, agentRepository)
	pnlUsecase := usecase.NewPnLUsecase(pnlRepository)
	nettingUsecase := usecase.NewNettingUsecase(
		zgClient,
		nettingRepository,
		agentRepository,
		nettingGateway,
		time.Duration(cfg.Netting.WindowSeconds)*time.Second,
	)
	nettingUsecase.Start(context.Background())

	// Handler Layer
	agentHandler := deliveryhttp.NewAgentHandler(agentUsecase)
	healthHandler := deliveryhttp.NewHealthHandler(healthUsecase)
	workOrderHandler := deliveryhttp.NewWorkOrderHandler(workOrderUsecase)
	ledgerHandler := deliveryhttp.NewLedgerHandler(pnlUsecase)
	storageHandler := deliveryhttp.NewStorageHandler(zgClient)
	nettingHandler := deliveryhttp.NewNettingHandler(nettingUsecase)
	quickNodeWebhookHandler := deliveryhttp.NewQuickNodeWebhookHandler(
		cfg.QuickNodeWebhookSecret,
		cfg.EscrowContractAddress,
		workOrderUsecase,
	)

	//Router
	router := deliveryhttp.NewRouter(
		healthHandler,
		workOrderHandler,
		quickNodeWebhookHandler,
		storageHandler,
		ledgerHandler,
		nettingHandler,
		agentHandler,
		cfg.AllowedCORSOrigins(),
	)

	log.Println("Starting application...")

	return &App{Config: cfg, Router: router}, nil
}
