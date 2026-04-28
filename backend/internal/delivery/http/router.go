package http

import "github.com/go-chi/chi/v5"

func NewRouter(healthHandler *HealthHandler, workOrderHandler *WorkOrderHandler, quickNodeWebhookHandler *QuickNodeWebhookHandler) *chi.Mux {
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		r.Post("/work-orders", workOrderHandler.Create)
		r.Post("/webhooks/quicknode/escrow-events", quickNodeWebhookHandler.HandleEscrowEvents)

		r.Route("/health", func(r chi.Router) {
			r.Get("/", healthHandler.Check)
			r.Post("/zgstorage", healthHandler.CheckUploadZGStorage)
		})

	})

	return r
}
