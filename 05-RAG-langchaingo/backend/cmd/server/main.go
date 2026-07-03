// Command server starts the RAG backend HTTP API.
//
//   go run ./cmd/server
//
// It wires config -> llm/embedder/milvus/duckdb -> rag engine -> chi router.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/api"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/config"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/embedder"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/llm"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/milvus"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/rag"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// --- connect dependencies ---
	emb := embedder.New(cfg.OllamaHost, cfg.OllamaEmbedModel)

	ms, err := milvus.New(cfg.MilvusAddr, cfg.MilvusCollectionDocs, cfg.MilvusCollectionQuotes)
	if err != nil {
		log.Fatalf("milvus: %v", err)
	}
	defer ms.Close()

	// Ensure collections exist (idempotent). On first run after `docker compose up`
	// this creates them; if Milvus isn't up yet, we warn but keep serving so
	// uploads/chat return a clear error instead of crashing the process.
	bootCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	if err := ms.EnsureCollections(bootCtx); err != nil {
		log.Printf("warning: ensure collections failed (Milvus may not be ready): %v", err)
	}
	cancel()

	db, err := store.New(cfg.DuckDBPath)
	if err != nil {
		log.Fatalf("duckdb: %v", err)
	}
	defer db.Close()

	llmCfg := llm.Config{
		Provider:    "deepseek",
		BaseURL:     cfg.DeepSeekBaseURL,
		APIKey:      cfg.DeepSeekAPIKey,
		Model:       cfg.DeepSeekModel,
		Temperature: 0.3,
	}
	lc, err := llm.NewClient(llmCfg)
	if err != nil {
		log.Fatalf("llm: %v", err)
	}

	engine := rag.NewEngine(lc, emb, ms, db)

	deps := &api.Deps{
		Cfg: cfg, Engine: engine, Milvus: ms, Store: db, Embedder: emb,
	}
	handler := api.Router(deps)

	srv := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// graceful shutdown
	go func() {
		fmt.Printf("RAG backend listening on http://localhost%s\n", cfg.ServerAddr)
		fmt.Printf("  CORS origins: %v\n", cfg.CORSOrigins)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	fmt.Println("\nshutting down...")
	ctx, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	_ = srv.Shutdown(ctx)
}
