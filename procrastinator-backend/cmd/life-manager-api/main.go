package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"procrastinator-backend/internal/config"
	"procrastinator-backend/internal/httpapi"
	"procrastinator-backend/internal/llm"
	"procrastinator-backend/internal/store"
)

func main() {
	// 1. Load .env file (tolerate missing file)
	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatalf("failed to load .env: %v", err)
		}
	}

	// 2. Load config
	cfg, err := config.Load(nil)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// 3. Create signal context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. Open DB pool
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	// 5. Run migrations
	if err := st.Migrate(ctx, "migrations"); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// 6. Ensure storage dir
	if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
		log.Fatalf("failed to create storage directory: %v", err)
	}

	// 7. Wire LLM client and HTTP server
	client := llm.New(*cfg)
	srv := httpapi.New(*cfg, st.Pool(), client)

	// 8. Serve with graceful shutdown
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: srv.Routes(),
	}

	go func() {
		log.Printf("starting server on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}

	if err := st.Close(); err != nil {
		log.Fatalf("failed to close database pool: %v", err)
	}

	log.Println("server stopped")
}
