package ingest

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"rsc.io/pdf"
)

// IngestPDF reads a PDF file and ingests its per-page text.
// Text runs on each page are sorted by Y (top-to-bottom) then X (left-to-right)
// so reading order is preserved for downstream chunking.
func IngestPDF(ctx context.Context, svc Services, source string, r io.ReaderAt, size int64) (int, error) {
	rd, err := pdf.NewReader(r, size)
	if err != nil {
		return 0, fmt.Errorf("pdf open: %w", err)
	}
	var all strings.Builder
	for i := 0; i < rd.NumPage(); i++ {
		page := rd.Page(i)
		content := page.Content()
		text := orderText(content.Text)
		if text != "" {
			all.WriteString(text)
			all.WriteString("\n\n")
		}
	}
	return IngestText(ctx, svc, source, "pdf", all.String())
}

// orderText sorts text runs by Y descending (PDF origin is bottom-left, so
// higher Y = higher on page) then X ascending, joining with spaces and
// inserting newlines when the Y gap is large enough to indicate a new line.
func orderText(ts []pdf.Text) string {
	if len(ts) == 0 {
		return ""
	}
	sort.SliceStable(ts, func(i, j int) bool {
		if ts[i].Y != ts[j].Y {
			return ts[i].Y > ts[j].Y // top of page first
		}
		return ts[i].X < ts[j].X
	})
	var b strings.Builder
	var lastY float64
	first := true
	for _, t := range ts {
		if !first && abs(t.Y-lastY) > 2 {
			b.WriteString("\n")
		} else if !first {
			b.WriteString(" ")
		}
		b.WriteString(t.S)
		lastY = t.Y
		first = false
	}
	return strings.TrimSpace(b.String())
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
