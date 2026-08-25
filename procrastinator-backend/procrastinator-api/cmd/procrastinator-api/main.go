package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"procrastinator-backend/commons-server/config"
	"procrastinator-backend/procrastinator-api"
	"procrastinator-backend/procrastinator-core/ingest"
	"procrastinator-backend/procrastinator-infra/filestorage"
	"procrastinator-backend/procrastinator-infra/llm"
	"procrastinator-backend/procrastinator-infra/postgres"
)

func main() {
	migrationsDir := flag.String("migrations", "migrations", "path to the database migrations directory")
	flag.Parse()

	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("loading .env: %v", err)
	}

	cfg, err := config.Load(nil)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx, *migrationsDir); err != nil {
		log.Fatalf("migrating database: %v", err)
	}

	if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
		log.Fatalf("creating storage dir %s: %v", cfg.StorageDir, err)
	}

	client := llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMTimeout)
	extractor := llm.NewExtractor(client)
	storage := filestorage.New(cfg.StorageDir)
	factory := postgres.NewRepoFactory(store.Pool())
	svc := ingest.New(factory, extractor, storage, cfg.MaxUploadBytes)
	server := api.New(svc, factory.AssetRepo(), factory.DocumentRepo(), cfg.MaxUploadBytes)

	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: server.Routes()}

	go func() {
		log.Printf("listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()

	log.Printf("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
