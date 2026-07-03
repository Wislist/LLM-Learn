// Command init_milvus connects to Milvus and creates the two collections.
// Run it once after `docker compose up -d`:
//
//	go run ./scripts/init_milvus
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/config"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/milvus"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := milvus.New(cfg.MilvusAddr, cfg.MilvusCollectionDocs, cfg.MilvusCollectionQuotes)
	if err != nil {
		log.Fatalf("milvus new: %v", err)
	}
	defer store.Close()

	if err := store.Ping(ctx); err != nil {
		log.Fatalf("milvus ping %s: %v\n(is `docker compose up -d` running?)", cfg.MilvusAddr, err)
	}
	fmt.Printf("✓ connected to Milvus at %s\n", cfg.MilvusAddr)

	if err := store.EnsureCollections(ctx); err != nil {
		log.Fatalf("ensure collections: %v", err)
	}
	fmt.Printf("✓ collections ready: %s, %s\n", cfg.MilvusCollectionDocs, cfg.MilvusCollectionQuotes)
}
