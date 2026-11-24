// Command chatty starts the Chatty AI chat server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"chatty/internal/api"
	"chatty/internal/chat"
	"chatty/internal/config"
	"chatty/internal/conversations"
	"chatty/internal/database"
	"chatty/internal/messages"
	"chatty/internal/mimir"
	"chatty/internal/observability"
	"chatty/internal/settings"
	"chatty/internal/statistics"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		// Final logger may be nil if config failed; fall back to stderr.
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

// healthcheck is a tiny self-probe used by the Docker HEALTHCHECK in the
// shell-less scratch image. It accepts HTTP 200 (healthy or degraded) as pass.
func healthcheck() int {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "3333"
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return 0
	}
	fmt.Fprintf(os.Stderr, "healthcheck: status %d\n", resp.StatusCode)
	return 1
}

func run() error {
	baseLog := observability.NewLogger(os.Getenv("LOG_LEVEL"), getenv("APP_ENV", "production"))

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := baseLog.With(
		slog.String("version", cfg.App.Version),
		slog.String("env", cfg.App.Env),
	)

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("starting chatty",
		slog.String("addr", cfg.App.Addr()),
		slog.String("mimir", cfg.Mimir.BaseURL),
	)

	// Database.
	db, err := database.Connect(rootCtx, cfg.Database, log)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(rootCtx); err != nil {
		return err
	}

	// Reap assistant messages left mid-flight by a previous run.
	if n, err := messages.NewRepo(db.Pool).MarkIncompleteAsFailed(rootCtx); err != nil {
		log.Warn("could not reap interrupted messages", slog.String("err", err.Error()))
	} else if n > 0 {
		log.Info("reaped interrupted assistant messages", slog.Int64("count", n))
	}

	// Mimir client + model discovery.
	backend := mimir.Backend(cfg.Mimir.Backend)
	client := mimir.NewClient(cfg.Mimir.BaseURL, cfg.Mimir.APIKey, cfg.Mimir.Timeout, backend)
	discoverMimir(rootCtx, client, cfg, log)

	// Wire services.
	convRepo := conversations.NewRepo(db.Pool)
	msgRepo := messages.NewRepo(db.Pool)
	statsSvc := statistics.NewService(db.Pool)
	setSvc := settings.NewService(db.Pool, cfg)
	engine := chat.NewEngine(convRepo, msgRepo, client, statsSvc, setSvc, cfg, log)

	srv := api.New(api.Deps{
		Config:   cfg,
		DB:       db,
		Conv:     convRepo,
		Msgs:     msgRepo,
		Mimir:    client,
		Stats:    statsSvc,
		Settings: setSvc,
		Engine:   engine,
		Logger:   log,
	})

	httpSrv := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0,  // streaming: rely on per-request timeouts
		WriteTimeout:      0,  // streaming responses are long-lived
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", cfg.App.Addr()))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-rootCtx.Done():
		log.Info("shutdown signal received, draining")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Warn("graceful shutdown timed out", slog.String("err", err.Error()))
		_ = httpSrv.Close()
	}
	log.Info("chatty stopped")
	return nil
}

// discoverMimir resolves the effective backend, model id and context window.
func discoverMimir(ctx context.Context, client *mimir.Client, cfg *config.Config, log *slog.Logger) {
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	info, err := client.Discover(dctx)
	if err != nil {
		log.Warn("mimir discovery failed; using configured model",
			slog.String("err", err.Error()),
			slog.String("model", cfg.Mimir.Model))
		if cfg.Mimir.Model != "" && !strings.EqualFold(cfg.Mimir.Model, "auto") {
			client.SetModel(cfg.Mimir.Model)
		}
		return
	}
	if cfg.Mimir.Model != "" && !strings.EqualFold(cfg.Mimir.Model, "auto") {
		client.SetModel(cfg.Mimir.Model)
	}
	if info.ContextLen > 0 {
		client.SetContextLen(info.ContextLen)
	}
	log.Info("mimir ready",
		slog.String("model", client.Model()),
		slog.String("backend", string(client.Backend())),
		slog.Int("context", client.ContextLen()),
	)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
