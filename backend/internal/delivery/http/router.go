package http

import "github.com/go-chi/chi/v5"

func NewRouter(healthHandler *HealthHandler, workOrderHandler *WorkOrderHandler) *chi.Mux {
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		r.Post("/work-orders", workOrderHandler.Create)

		r.Route("/health", func(r chi.Router) {
			r.Get("/", healthHandler.Check)
			r.Post("/zgstorage", healthHandler.CheckUploadZGStorage)
		})

	})

	return r
}
