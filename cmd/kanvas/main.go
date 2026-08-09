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

	"github.com/hkjang/kanvas/internal/api"
	"github.com/hkjang/kanvas/internal/auth"
	"github.com/hkjang/kanvas/internal/buildinfo"
	"github.com/hkjang/kanvas/internal/config"
	"github.com/hkjang/kanvas/internal/migration"
	"github.com/hkjang/kanvas/internal/security"
	"github.com/hkjang/kanvas/internal/store"
	"github.com/hkjang/kanvas/webembed"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("database startup failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	if err = st.Migrate(ctx); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	if err = st.BootstrapAdmin(ctx, cfg.BootstrapAdmin, cfg.BootstrapPassword); err != nil {
		logger.Error("bootstrap admin failed", "error", err)
		os.Exit(1)
	}
	if err = st.EnsureWelcome(ctx, cfg.BootstrapAdmin); err != nil {
		logger.Error("welcome content failed", "error", err)
		os.Exit(1)
	}
	vault, err := security.OpenVault(cfg.DataDir)
	if err != nil {
		logger.Error("secret vault startup failed", "error", err)
		os.Exit(1)
	}
	authManager := &auth.Manager{Store: st, Vault: vault}
	migrationService := &migration.Service{Store: st}
	handler := api.New(st, authManager, migrationService, vault, cfg.ConfluenceDSN, webembed.Handler(), logger)
	server := &http.Server{Addr: cfg.ListenAddress, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		logger.Info("Kanvas started", "version", buildinfo.Version, "address", cfg.ListenAddress)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()
	<-done
	logger.Info("Kanvas shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
