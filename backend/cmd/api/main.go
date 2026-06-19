package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harundarat/rive/backend/internal/app"
)

const (
	// readHeaderTimeout is the primary guard against Slowloris-style attacks:
	// clients must send the full request header within this window.
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	// writeTimeout is intentionally generous: work order creation currently
	// performs a synchronous 0G upload inside the request (M1). It will be
	// tightened in Fase 3 once that upload moves out of the request path.
	writeTimeout = 120 * time.Second
	idleTimeout  = 120 * time.Second
	// shutdownTimeout bounds how long we wait for in-flight requests to drain.
	shutdownTimeout = 15 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	application, err := app.Initialize()
	if err != nil {
		slog.Error("error initializing application", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	server := &http.Server{
		Addr:              ":8080",
		Handler:           application.Router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server running", "addr", ":8080")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		slog.Error("error starting server", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
	// application.Close() runs via defer after Shutdown drains in-flight requests,
	// releasing the netting loop, settlement gateway, 0G client, and DB pool.
}
