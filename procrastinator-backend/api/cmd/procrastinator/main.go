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

	"procrastinator-backend/api"
	"procrastinator-backend/config"
	"procrastinator-backend/core/household"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/lifecycle"
	"procrastinator-backend/core/processing"
	"procrastinator-backend/core/review"
	"procrastinator-backend/core/search"
	"procrastinator-backend/core/statement"
	"procrastinator-backend/infra/filestorage"
	"procrastinator-backend/infra/llm"
	"procrastinator-backend/infra/pdftext"
	"procrastinator-backend/infra/postgres"
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
	chatter := llm.NewClientChatter(client)
	// Extraction workers come from config (default: two workers on the LLM
	// model with the extract/verify strategies, i.e. dual consensus).
	workers := make([]processing.Worker, 0, len(cfg.LLMWorkers))
	for _, w := range cfg.LLMWorkers {
		workers = append(workers, processing.Worker{
			Model:    w.Model,
			BaseURL:  w.BaseURL,
			Strategy: processing.Strategy{Name: w.Strategy},
		})
	}
	// An optional brand-lexicon override (PROCRASTINATOR_BRAND_LEXICON) replaces
	// the embedded canonical list used by consensus to validate/correct brands.
	if len(cfg.BrandLexicon) > 0 {
		processing.SetDefaultBrandLexicon(processing.NewBrandLexiconFromList(cfg.BrandLexicon))
	}
	extractor := processing.NewExtractor(chatter, workers, cfg.LLMTimeout, llm.SystemPrompt())
	storage := filestorage.New(cfg.StorageDir)
	statementStore := filestorage.NewStatement(cfg.StorageDir)
	factory := postgres.NewFactory(store.Pool())
	movRepo := postgres.NewMovementRepository(store.Pool())
	docRepo := postgres.NewDocumentRepository(store.Pool())
	pdfExtractor := pdftext.New()
	ledgerSvc := ledger.New(factory, movRepo)
	reviewSvc := review.New(factory)
	searchSvc := search.New(factory.Search)
	svc := processing.New(factory, extractor, storage, cfg.MaxUploadBytes, cfg.IngestReviewThreshold, reviewSvc, cfg.AssetDeleteRetentionDays, cfg.LookupCandidateLimit, cfg.ProcessTimeout)
	svc.SetTextStorage(filestorage.NewText(cfg.StorageDir))
	lifecycleSvc := lifecycle.New(factory, cfg.AssetDeleteRetentionDays)
	statementSvc := statement.New(factory, statementStore, movRepo, docRepo, pdfExtractor, cfg.MaxStatementBytes, cfg.MaxStatementLines)
	householdSvc := household.New(factory)
	server := api.New(svc, factory, ledgerSvc, movRepo, cfg.MaxUploadBytes, statementSvc, cfg.MaxStatementBytes, householdSvc, searchSvc, reviewSvc, lifecycleSvc)

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
