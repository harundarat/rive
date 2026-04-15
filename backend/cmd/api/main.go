package main

import (
	"log"
	"net/http"

	"github.com/harundarat/rive/backend/internal/app"
)

func main() {
	application, err := app.Initialize()
	if err != nil {
		log.Fatalf("Error initializing application: %v", err)
	}

	_ = application

	server := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello world"))
		}),
	}

	log.Println("Server running on :8080")
	err = server.ListenAndServe()
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
