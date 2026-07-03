// Package config loads runtime configuration from environment.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime settings.
type Config struct {
	// LLM (DeepSeek, OpenAI-compatible).
	DeepSeekAPIKey  string
	DeepSeekBaseURL string
	DeepSeekModel   string

	// Embeddings via Ollama.
	OllamaHost      string
	OllamaEmbedModel string
	EmbedDim        int // bge-m3 = 1024

	// Milvus.
	MilvusAddr             string
	MilvusCollectionDocs    string
	MilvusCollectionQuotes  string

	// DuckDB.
	DuckDBPath string

	// Server.
	ServerAddr   string
	CORSOrigins  []string

	// Uploads.
	UploadDir    string
	MaxUploadMB  int64
}

// Load reads .env (if present) and environment variables.
func Load() (*Config, error) {
	// .env is optional; in production use real env vars.
	_ = godotenv.Load()

	cfg := &Config{
		DeepSeekAPIKey:  os.Getenv("DEEPSEEK_API_KEY"),
		DeepSeekBaseURL: getenv("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1"),
		DeepSeekModel:   getenv("DEEPSEEK_MODEL", "deepseek-chat"),

		OllamaHost:       getenv("OLLAMA_HOST", "http://localhost:11434"),
		OllamaEmbedModel: getenv("OLLAMA_EMBED_MODEL", "bge-m3"),
		EmbedDim:         1024,

		MilvusAddr:            getenv("MILVUS_ADDR", "localhost:19530"),
		MilvusCollectionDocs:  getenv("MILVUS_COLLECTION_DOCS", "rag_documents"),
		MilvusCollectionQuotes: getenv("MILVUS_COLLECTION_QUOTES", "rag_quotes"),

		DuckDBPath: getenv("DUCKDB_PATH", "./data/app.duckdb"),

		ServerAddr:  getenv("SERVER_ADDR", ":8787"),
		CORSOrigins: splitCSV(getenv("CORS_ORIGINS", "http://localhost:3000")),

		UploadDir:   getenv("UPLOAD_DIR", "./data/uploads"),
		MaxUploadMB: getenvInt("MAX_UPLOAD_MB", 50),
	}

	if cfg.DeepSeekAPIKey == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY is required (set in .env)")
	}

	// ensure dirs
	for _, d := range []string{cfg.UploadDir, filepath.Dir(cfg.DuckDBPath)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
