// Command api is the entry point of the ThaiMart User Management API.
// It wires the adapters together, serves HTTP and shuts down gracefully
// on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/platform/httpapi"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited with error", "err", err)
		os.Exit(1)
	}
	slog.Info("api shut down cleanly")
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	uri := envOr("MONGO_URI", "mongodb://localhost:27017")
	addr := envOr("HTTP_ADDR", ":8080")

	// The v2 driver connects lazily; the ping is what actually proves the
	// URI works, so a misconfigured environment fails fast at startup.
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("creating mongo client: %w", err)
	}
	pingCtx, cancelPing := context.WithTimeout(ctx, 10*time.Second)
	defer cancelPing()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		return fmt.Errorf("pinging mongo at %s: %w", uri, err)
	}
	slog.Info("mongo connected", "uri", uri)
	defer client.Disconnect(context.Background())

	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewRouter(mongoPinger{client}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// mongoPinger adapts *mongo.Client to the app.HealthChecker port so the
// HTTP layer stays free of MongoDB imports.
type mongoPinger struct{ client *mongo.Client }

func (p mongoPinger) Ping(ctx context.Context) error {
	return p.client.Ping(ctx, readpref.Primary())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
