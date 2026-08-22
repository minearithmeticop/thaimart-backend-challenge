// Command api is the entry point of the ThaiMart User Management API.
// It wires the adapters together, serves HTTP and gRPC, and shuts down
// gracefully on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"google.golang.org/grpc"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/auth"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/report"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/user"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/config"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/platform/grpcapi"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/platform/httpapi"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/platform/mongostore"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/platform/security"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited with error", "err", err)
		os.Exit(1)
	}
	slog.Info("api shut down cleanly")
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogFormat)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ---- Driven adapters --------------------------------------------------

	// The v2 driver connects lazily; the ping is what actually proves the
	// URI works, so a misconfigured environment fails fast at startup.
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return fmt.Errorf("creating mongo client: %w", err)
	}
	pingCtx, cancelPing := context.WithTimeout(ctx, 10*time.Second)
	defer cancelPing()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		return fmt.Errorf("pinging mongo at %s: %w", cfg.MongoURI, err)
	}
	defer client.Disconnect(context.Background())

	indexCtx, cancelIndex := context.WithTimeout(ctx, 10*time.Second)
	defer cancelIndex()
	repo, err := mongostore.NewUserRepository(indexCtx, client.Database(cfg.MongoDB))
	if err != nil {
		return err
	}

	hasher := security.NewBcryptHasher(cfg.BcryptCost)
	tokens := security.NewJWTManager(cfg.JWTSecret, cfg.JWTTTL)

	// ---- Application core ---------------------------------------------------

	users := user.NewService(repo, hasher, logger)
	authSvc := auth.NewService(users, repo, hasher, tokens, logger)

	// ---- Background jobs -----------------------------------------------------

	// Logs the total user count once per interval; ctx cancellation on
	// shutdown stops it.
	go report.NewUserCountReporter(repo, cfg.ReportEvery, logger).Run(ctx)

	// ---- Driving adapters ----------------------------------------------------

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(mongoPinger{client}, tokens, authSvc, users, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listening on grpc %s: %w", cfg.GRPCAddr, err)
	}
	grpcSrv := grpcapi.NewServer(users, tokens, logger)

	errCh := make(chan error, 2)
	go func() {
		slog.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http: %w", err)
		}
	}()
	go func() {
		slog.Info("grpc server listening", "addr", cfg.GRPCAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("grpc: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownHTTP(httpSrv)
		shutdownGRPC(grpcSrv)
		return nil
	}
}

func shutdownHTTP(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("http shutdown failed", "err", err)
	}
}

func shutdownGRPC(srv *grpc.Server) {
	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		srv.Stop() // in-flight RPCs got their chance; force-stop now
	}
}

// mongoPinger adapts *mongo.Client to the app.HealthChecker port so the
// HTTP layer stays free of MongoDB imports.
type mongoPinger struct{ client *mongo.Client }

func (p mongoPinger) Ping(ctx context.Context) error {
	return p.client.Ping(ctx, readpref.Primary())
}

func newLogger(format string) *slog.Logger {
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}
