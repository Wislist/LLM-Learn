// Package ingest parses uploaded documents (docx/pdf/xlsx) into text chunks,
// embeds them via the embedder, and inserts them into Milvus.
//
// Chunking strategy:
//   - docx/pdf: split text on paragraph boundaries, then greedily pack into
//     ~512-token windows with a 1-paragraph overlap. We approximate "token"
//     by rune count (Chinese ~1 rune per token; English ~4 chars per token).
//   - excel narrative: each non-empty cell becomes a short chunk; adjacent
//     cells in the same row are joined so context (project + item) survives.
//   - excel quotes: each line item becomes a textualized row chunk.
package ingest

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/embedder"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/milvus"
)

// Services bundles the dependencies ingest needs.
type Services struct {
	Embedder *embedder.Embedder
	Milvus   *milvus.Store
}

const (
	// TargetChunkRunes ≈ 512 tokens worth of Chinese/English mixed text.
	TargetChunkRunes = 800
	OverlapRunes     = 120
)

// ChunkText splits a long text into overlapping windows.
func ChunkText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	// Split into paragraphs first to avoid cutting mid-sentence where possible.
	paras := splitParagraphs(text)

	var chunks []string
	var cur strings.Builder
	curRunes := 0
	for _, p := range paras {
		pr := utf8.RuneCountInString(p)
		// If a single paragraph already exceeds the target, hard-split it.
		if pr > TargetChunkRunes {
			if cur.Len() > 0 {
				chunks = append(chunks, cur.String())
				cur.Reset()
				curRunes = 0
			}
			chunks = append(chunks, hardSplit(p, TargetChunkRunes, OverlapRunes)...)
			continue
		}
		if curRunes+pr > TargetChunkRunes && cur.Len() > 0 {
			chunks = append(chunks, cur.String())
			// carry overlap from the tail of the current buffer
			tail := tailRunes(cur.String(), OverlapRunes)
			cur.Reset()
			cur.WriteString(tail)
			curRunes = utf8.RuneCountInString(tail)
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(p)
		curRunes += pr
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

func splitParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func hardSplit(s string, size, overlap int) []string {
	runes := []rune(s)
	var out []string
	for i := 0; i < len(runes); i += size - overlap {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
		if end == len(runes) {
			break
		}
	}
	return out
}

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

// ---------- embed + insert helpers ----------

// embedChunks embeds each chunk and returns DocRow ready for Milvus.
func embedChunks(ctx context.Context, e *embedder.Embedder, source, docType string, chunks []string) ([]milvus.DocRow, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	vecs, err := e.EmbedBatch(ctx, chunks)
	if err != nil {
		return nil, fmt.Errorf("embed chunks: %w", err)
	}
	rows := make([]milvus.DocRow, len(chunks))
	for i, c := range chunks {
		rows[i] = milvus.DocRow{
			Source:  source,
			Type:    docType,
			Content: c,
			Vector:  vecs[i],
		}
	}
	return rows, nil
}

// IngestText is the generic path for docx/pdf/plaintext narrative content.
func IngestText(ctx context.Context, svc Services, source, docType, text string) (int, error) {
	chunks := ChunkText(text)
	rows, err := embedChunks(ctx, svc.Embedder, source, docType, chunks)
	if err != nil {
		return 0, err
	}
	if err := svc.Milvus.InsertDocs(ctx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}
