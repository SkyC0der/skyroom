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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SkyC0der/skyroom/database/migrations"
	"github.com/SkyC0der/skyroom/internal/community"
	"github.com/SkyC0der/skyroom/internal/conversation"
	"github.com/SkyC0der/skyroom/internal/identity"
	"github.com/SkyC0der/skyroom/internal/platform/httpserver"
	platformpostgres "github.com/SkyC0der/skyroom/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	addr := envOrDefault("HTTP_ADDR", ":8080")
	appEnv := envOrDefault("APP_ENV", "development")
	authSecret := envOrDefault("AUTH_HASH_SECRET", "development-only-change-me")
	if appEnv != "development" && len(authSecret) < 32 {
		logger.Error("AUTH_HASH_SECRET must contain at least 32 bytes outside development")
		os.Exit(1)
	}
	identityStorage, communityStorage, conversationStorage, closeStores, err := applicationStores(logger, appEnv, os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error("initialize identity store", "error", err)
		os.Exit(1)
	}
	defer closeStores()
	auth := identity.NewService(
		identityStorage,
		identity.SMTPCodeSender{
			Address: envOrDefault("SMTP_ADDR", "localhost:1026"),
			From:    envOrDefault("EMAIL_FROM", "hello@rooms.studioeco.co"),
		},
		authSecret,
	)
	communities := community.NewService(communityStorage, auth)
	conversations := conversation.NewService(conversationStorage, auth)

	server := &http.Server{
		Addr: addr,
		Handler: httpserver.New(
			logger,
			envOrDefault("WEB_ORIGIN", "http://localhost:3000"),
			httpserver.WithIdentity(auth, appEnv != "development"),
			httpserver.WithCommunity(communities),
			httpserver.WithConversation(conversations),
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("http server listening", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown requested")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}

func applicationStores(logger *slog.Logger, appEnv, databaseURL string) (identity.Store, community.Store, conversation.Store, func(), error) {
	if databaseURL == "" {
		if appEnv != "development" {
			return nil, nil, nil, func() {}, errors.New("DATABASE_URL is required outside development")
		}
		logger.Warn("DATABASE_URL is empty; application state will be in memory")
		communityStore := community.NewMemoryStore()
		return identity.NewMemoryStore(), communityStore, conversation.NewMemoryStore(communityStore), func() {}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, nil, nil, func() {}, err
	}
	ran, err := platformpostgres.Migrate(ctx, connection, migrations.FS)
	_ = connection.Close(ctx)
	if err != nil {
		return nil, nil, nil, func() {}, err
	}
	if len(ran) > 0 {
		logger.Info("database migrations applied", "migrations", ran)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, nil, nil, func() {}, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, nil, func() {}, err
	}
	return identity.NewPostgresStore(pool), community.NewPostgresStore(pool), conversation.NewPostgresStore(pool), pool.Close, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
