package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dibou/mcp-obsidian/internal/config"
	"github.com/dibou/mcp-obsidian/internal/httpserver"
	syncer "github.com/dibou/mcp-obsidian/internal/sync"
	s3sync "github.com/dibou/mcp-obsidian/internal/sync/s3"
	"github.com/dibou/mcp-obsidian/internal/vault"
)

const version = "0.1.0"

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.LUTC | log.Lmsgprefix)
	log.SetPrefix("mcp-obsidian: ")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	v, err := vault.New(cfg.VaultPath, cfg.AllowDelete)
	if err != nil {
		log.Fatal(err)
	}

	var s syncer.Syncer = syncer.NewNoop()
	if cfg.S3.Enabled {
		s, err = s3sync.New(context.Background(), cfg.S3, v)
		if err != nil {
			log.Fatal(err)
		}
		logger.Info("S3 sync enabled", "bucket", cfg.S3.Bucket, "prefix", cfg.S3.Prefix, "region", cfg.S3.Region, "interval", cfg.S3.SyncInterval.String())
	} else {
		logger.Info("S3 sync disabled")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := httpserver.New(ctx, httpserver.Dependencies{
		Config:  cfg,
		Vault:   v,
		Sync:    s,
		Logger:  logger,
		Version: version,
	})
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown failed: %v", err)
		}
	}()

	logger.Info("listening", "addr", cfg.HTTP.Addr, "public_base_url", cfg.HTTP.PublicBaseURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
