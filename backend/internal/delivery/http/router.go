package http

import "github.com/go-chi/chi/v5"

func NewRouter(
	healthHandler *HealthHandler,
	workOrderHandler *WorkOrderHandler,
	quickNodeWebhookHandler *QuickNodeWebhookHandler,
	storageHandler *StorageHandler,
	ledgerHandler *LedgerHandler,
	nettingHandler *NettingHandler,
	agentHandler *AgentHandler,
	corsAllowedOrigins []string,
) *chi.Mux {
	r := chi.NewRouter()
	r.Use(corsMiddleware(corsAllowedOrigins))

	r.Route("/api", func(r chi.Router) {
		r.Post("/agents/onboard", agentHandler.Onboard)
		r.Post("/work-orders", workOrderHandler.Create)
		r.Get("/work-orders/{onchainOrderID}", workOrderHandler.Get)
		r.Post("/work-orders/{onchainOrderID}/delivery", workOrderHandler.SubmitDelivery)
		r.Get("/ledger/{walletAddress}/pnl", ledgerHandler.GetPnL)
		r.Post("/payments/intent", nettingHandler.CreatePaymentIntent)
		r.Post("/storage/upload", storageHandler.Upload)
		r.Post("/webhooks/quicknode/escrow-events", quickNodeWebhookHandler.HandleEscrowEvents)

		r.Route("/health", func(r chi.Router) {
			r.Get("/", healthHandler.Check)
			r.Post("/zgstorage", healthHandler.CheckUploadZGStorage)
		})

	})

	return r
}
