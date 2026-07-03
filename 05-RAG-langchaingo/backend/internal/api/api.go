// Package api exposes the HTTP handlers for the RAG backend.
//
// Endpoints
//   GET  /healthz
//   POST /api/chat          -> SSE stream of answer deltas + final metadata JSON
//   POST /api/upload        -> upload docx/pdf/xlsx for ingestion
//   GET  /api/schema/sales  -> returns the sales table schema (for debugging)
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/config"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/embedder"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/ingest"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/milvus"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/rag"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/store"
)

// Deps bundles everything the handlers need.
type Deps struct {
	Cfg      *config.Config
	Engine   *rag.Engine
	Milvus   *milvus.Store
	Store    *store.Store
	Embedder *embedder.Embedder
}

// Router builds the chi router with all handlers + CORS.
func Router(d *Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   d.Cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", healthz(d))
	r.Post("/api/chat", chat(d))
	r.Post("/api/upload", upload(d))
	r.Get("/api/schema/sales", salesSchema(d))
	return r
}

// ---------- health ----------

func healthz(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), shortTimeout)
		defer cancel()
		status := map[string]string{
			"server": "ok",
		}
		if err := d.Embedder.Ping(ctx); err != nil {
			status["ollama"] = "err: " + err.Error()
		} else {
			status["ollama"] = "ok"
		}
		if err := d.Milvus.Ping(ctx); err != nil {
			status["milvus"] = "err: " + err.Error()
		} else {
			status["milvus"] = "ok"
		}
		writeJSON(w, http.StatusOK, status)
	}
}

// ---------- chat (SSE) ----------

type chatRequest struct {
	Question string `json:"question"`
	Session  string `json:"session"`
}

func chat(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.Question) == "" {
			writeJSON(w, 400, map[string]string{"error": "question is empty"})
			return
		}
		if req.Session == "" {
			req.Session = "default"
		}

		// log user message
		_ = d.Store.LogChat(r.Context(), "user", req.Question, req.Session)

		// Set up SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, 500, map[string]string{"error": "streaming unsupported"})
			return
		}

		ctx, cancel := rag.EnsureCtxTimeout(r.Context())
		defer cancel()

		onDelta := func(chunk string) {
			sseEvent(w, "delta", map[string]string{"text": chunk})
			flusher.Flush()
		}

		ans, err := d.Engine.AskStream(ctx, req.Question, onDelta)
		if err != nil {
			sseEvent(w, "error", map[string]string{"message": err.Error()})
			flusher.Flush()
			return
		}

		// final metadata (context hits + chart spec)
		sseEvent(w, "done", ans)
		flusher.Flush()

		// log assistant answer
		_ = d.Store.LogChat(r.Context(), "assistant", ans.Text, req.Session)
	}
}

// ---------- upload ----------

func upload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		maxBytes := d.Cfg.MaxUploadMB * 1024 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			writeJSON(w, 400, map[string]string{"error": "parse form: " + err.Error()})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "no file field"})
			return
		}
		defer file.Close()

		// Read into memory (we control max upload size). For very large files
		// you'd stream to disk first; MVP keeps it simple.
		data, err := io.ReadAll(file)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "read: " + err.Error()})
			return
		}
		ext := strings.ToLower(filepath.Ext(header.Filename))
		src := header.Filename
		svc := ingest.Services{Embedder: d.Embedder, Milvus: d.Milvus}

		ctx, cancel := rag.EnsureCtxTimeout(r.Context())
		defer cancel()

		var result ingestResult
		switch ext {
		case ".docx":
			n, err := ingest.IngestDocx(ctx, svc, src, bytesReaderAt(data), int64(len(data)))
			result = ingestResult{Kind: "docx", Chunks: n, Err: errString(err)}
		case ".pdf":
			n, err := ingest.IngestPDF(ctx, svc, src, bytesReaderAt(data), int64(len(data)))
			result = ingestResult{Kind: "pdf", Chunks: n, Err: errString(err)}
		case ".xlsx", ".xls":
			kind, n, err := ingest.IngestExcelRoutes(ctx, svc, d.Store, src, strings.NewReader(string(data)))
			result = ingestResult{Kind: excelKindName(kind), Rows: n, Err: errString(err)}
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported extension: " + ext})
			return
		}
		writeJSON(w, 200, result)
	}
}

type ingestResult struct {
	Kind   string `json:"kind"`
	Chunks int    `json:"chunks,omitempty"`
	Rows   int    `json:"rows,omitempty"`
	Err    string `json:"error,omitempty"`
}

// ---------- sales schema ----------

func salesSchema(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{
			"table":  "sales",
			"columns": "order_date DATE, product VARCHAR, region VARCHAR, qty DOUBLE, amount DOUBLE, source VARCHAR",
		})
	}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func sseEvent(w http.ResponseWriter, event string, data any) {
	payload, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func excelKindName(k ingest.ExcelKind) string {
	switch k {
	case ingest.ExcelSales:
		return "sales"
	case ingest.ExcelQuotes:
		return "quotes"
	case ingest.ExcelNarrative:
		return "narrative"
	default:
		return "unknown"
	}
}

// bytesReaderAt wraps a []byte to satisfy io.ReaderAt (needed by zip/pdf).
type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	return n, nil
}

const shortTimeout = 5 * time.Second
