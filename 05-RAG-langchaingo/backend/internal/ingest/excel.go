package ingest

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/milvus"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/store"
)

// ExcelKind classifies an uploaded .xlsx by its header row.
type ExcelKind int

const (
	ExcelUnknown ExcelKind = iota
	ExcelSales   // columns: order_date, product, region, qty, amount
	ExcelQuotes  // columns: project_name, item_name, qty, unit_price, total
	ExcelNarrative
)

// ClassifyExcel peeks at the first sheet's header row to decide routing.
func ClassifyExcel(r io.Reader) (ExcelKind, []string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return ExcelUnknown, nil, fmt.Errorf("excel open: %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return ExcelUnknown, nil, fmt.Errorf("excel: no sheets")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return ExcelUnknown, nil, fmt.Errorf("excel rows: %w", err)
	}
	if len(rows) == 0 {
		return ExcelUnknown, nil, fmt.Errorf("excel: empty first sheet")
	}
	headers := normalizeHeaders(rows[0])
	hset := toSet(headers)
	switch {
	case hasAll(hset, "order_date", "product", "qty"):
		return ExcelSales, headers, nil
	case hasAll(hset, "project_name", "item_name", "unit_price"):
		return ExcelQuotes, headers, nil
	}
	return ExcelNarrative, headers, nil
}

// IngestExcelRoutes routes an xlsx to the right pipeline based on classification.
// For sales sheets it loads into DuckDB; for quote sheets it embeds rows into
// the quotes collection; for narrative sheets it ingests cell text as documents.
// It returns (kind, rowsAffected, error).
func IngestExcelRoutes(ctx context.Context, svc Services, db *store.Store, source string, r io.Reader) (ExcelKind, int, error) {
	// excelize needs a fresh reader each call; we already consumed r in
	// ClassifyExcel in the API layer, so callers pass a re-opened reader here.
	f, err := excelize.OpenReader(r)
	if err != nil {
		return ExcelUnknown, 0, fmt.Errorf("excel open: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return ExcelUnknown, 0, fmt.Errorf("excel: no sheets")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return ExcelUnknown, 0, fmt.Errorf("excel rows: %w", err)
	}
	if len(rows) < 2 {
		return ExcelNarrative, 0, nil
	}
	headers := normalizeHeaders(rows[0])
	hset := toSet(headers)
	body := rows[1:]

	switch {
	case hasAll(hset, "order_date", "product", "qty"):
		// Sales: rebuild an in-memory normalized sheet for DuckDB? We can't
		// pass the original path (we got a reader). Write a temp xlsx? Simpler:
		// the API handler saves uploads to disk first, so IngestSalesFromPath
		// is the real entry. Here we just ingest the text as narrative too.
		return ExcelSales, len(body), ingestSalesRows(ctx, db, headers, body, source)
	case hasAll(hset, "project_name", "item_name", "unit_price"):
		return ExcelQuotes, len(body), ingestQuoteRows(ctx, svc, headers, body, source)
	default:
		return ExcelNarrative, len(body), ingestNarrativeRows(ctx, svc, headers, body, source)
	}
}

// ingestSalesRows streams rows into DuckDB one INSERT per row.
// (When the upload has already been saved to disk, prefer store.LoadSalesFromExcel.)
func ingestSalesRows(ctx context.Context, db *store.Store, headers []string, rows [][]string, source string) error {
	idx := indexMap(headers)
	stmt := `INSERT INTO sales (order_date, product, region, qty, amount, source) VALUES (?, ?, ?, ?, ?, ?)`
	for _, r := range rows {
		date := cellAt(r, idx["order_date"])
		if date == "" {
			continue
		}
		_, err := db.DB().ExecContext(ctx, stmt,
			date,
			cellAt(r, idx["product"]),
			cellAt(r, idx["region"]),
			parseFloat(cellAt(r, idx["qty"])),
			parseFloat(cellAt(r, idx["amount"])),
			source,
		)
		if err != nil {
			return fmt.Errorf("sales insert: %w", err)
		}
	}
	return nil
}

// ingestQuoteRows textualizes each row and embeds into the quotes collection.
func ingestQuoteRows(ctx context.Context, svc Services, headers []string, rows [][]string, source string) error {
	idx := indexMap(headers)
	var mrows []milvus.QuoteRow
	texts := make([]string, 0, len(rows))
	for _, r := range rows {
		project := cellAt(r, idx["project_name"])
		item := cellAt(r, idx["item_name"])
		qty := cellAt(r, idx["qty"])
		unit := cellAt(r, idx["unit_price"])
		total := cellAt(r, idx["total"])
		if project == "" && item == "" {
			continue
		}
		text := fmt.Sprintf("项目: %s | 品名: %s | 数量: %s | 单价: %s | 总价: %s",
			project, item, qty, unit, total)
		mrows = append(mrows, milvus.QuoteRow{
			Project: project, Item: item, Qty: qty,
			Unit: unit, Total: total, Source: source, Content: text,
		})
		texts = append(texts, text)
	}
	if len(texts) == 0 {
		return nil
	}
	vecs, err := svc.Embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed quotes: %w", err)
	}
	for i := range mrows {
		mrows[i].Vector = vecs[i]
	}
	return svc.Milvus.InsertQuotes(ctx, mrows)
}

// ingestNarrativeRows joins each row's cells into a paragraph and chunks it.
func ingestNarrativeRows(ctx context.Context, svc Services, headers []string, rows [][]string, source string) error {
	var b strings.Builder
	b.WriteString(strings.Join(headers, " | "))
	b.WriteString("\n")
	for _, r := range rows {
		// pad row to header length
		for i, h := range headers {
			val := ""
			if i < len(r) {
				val = strings.TrimSpace(r[i])
			}
			if val == "" {
				continue
			}
			b.WriteString(h)
			b.WriteString(": ")
			b.WriteString(val)
			b.WriteString("  ")
		}
		b.WriteString("\n")
	}
	_, err := IngestText(ctx, svc, source, "xlsx", b.String())
	return err
}

// ---------- small helpers ----------

func normalizeHeaders(row []string) []string {
	out := make([]string, len(row))
	for i, h := range row {
		h = strings.ToLower(strings.TrimSpace(h))
		h = strings.ReplaceAll(h, " ", "_")
		out[i] = h
	}
	return out
}

func toSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		if x != "" {
			m[x] = true
		}
	}
	return m
}

func hasAll(set map[string]bool, keys ...string) bool {
	for _, k := range keys {
		if !set[k] {
			return false
		}
	}
	return true
}

func indexMap(headers []string) map[string]int {
	m := make(map[string]int, len(headers))
	for i, h := range headers {
		m[h] = i
	}
	return m
}

func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
