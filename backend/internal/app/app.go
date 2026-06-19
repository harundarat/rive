package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/harundarat/rive/backend/internal/config"
	deliveryhttp "github.com/harundarat/rive/backend/internal/delivery/http"
	"github.com/harundarat/rive/backend/internal/infrastructure/database"
	"github.com/harundarat/rive/backend/internal/infrastructure/settlement"
	"github.com/harundarat/rive/backend/internal/infrastructure/storage"
	postgresrepo "github.com/harundarat/rive/backend/internal/repository/postgres"
	"github.com/harundarat/rive/backend/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	Config *config.Config
	Router http.Handler

	pgDB           *pgxpool.Pool
	zgClient       *storage.ZGClient
	nettingGateway *settlement.NettingGateway
	nettingCancel  context.CancelFunc
}

// Close releases all long-lived resources held by the application. It stops the
// background netting loop first, then closes the settlement gateway, 0G storage
// client, and database pool. Safe to call once after Initialize succeeds.
func (a *App) Close() {
	if a.nettingCancel != nil {
		a.nettingCancel()
	}
	if a.nettingGateway != nil {
		a.nettingGateway.Close()
	}
	if a.zgClient != nil {
		a.zgClient.Close()
	}
	if a.pgDB != nil {
		a.pgDB.Close()
	}
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
	workOrderRepository := postgresrepo.NewWorkOrderRepository(pgDB)
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
	nettingCtx, nettingCancel := context.WithCancel(context.Background())
	if err := nettingUsecase.ReconcileStuckBatches(nettingCtx); err != nil {
		// Best-effort: a reconcile failure must not block startup.
		slog.Error("startup netting reconcile failed", "error", err)
	}
	nettingUsecase.Start(nettingCtx)

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

	slog.Info("starting application")

	return &App{
		Config:         cfg,
		Router:         router,
		pgDB:           pgDB,
		zgClient:       zgClient,
		nettingGateway: nettingGateway,
		nettingCancel:  nettingCancel,
	}, nil
}
