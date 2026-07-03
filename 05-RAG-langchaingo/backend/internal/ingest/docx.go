package ingest

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/embedder"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/milvus"
)

// IngestDocx reads a .docx file (which is a zip of XML) and ingests its text.
// We avoid a third-party docx library by parsing word/document.xml directly
// and concatenating every <w:t> text run, inserting paragraph breaks on <w:p>.
func IngestDocx(ctx context.Context, svc Services, source string, r io.ReaderAt, size int64) (int, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return 0, fmt.Errorf("docx open zip: %w", err)
	}
	var docXML []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return 0, fmt.Errorf("docx open document.xml: %w", err)
			}
			docXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return 0, fmt.Errorf("docx read document.xml: %w", err)
			}
			break
		}
	}
	if docXML == nil {
		return 0, fmt.Errorf("docx: word/document.xml not found (not a valid docx?)")
	}
	text := extractDocxText(docXML)
	return IngestText(ctx, svc, source, "docx", text)
}

// extractDocxText walks the OOXML body and emits text with paragraph breaks.
func extractDocxText(data []byte) string {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var b strings.Builder
	inParagraph := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			switch tt.Name.Local {
			case "p":
				inParagraph = true
			case "t":
				// text run — read its char data
				var s string
				if err := dec.DecodeElement(&s, &tt); err == nil {
					b.WriteString(s)
				}
			case "br", "tab":
				b.WriteString(" ")
			}
		case xml.EndElement:
			if tt.Name.Local == "p" && inParagraph {
				b.WriteString("\n")
				inParagraph = false
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// Compile-time asserts that Services stays compatible if we re-export helpers.
var _ = (*embedder.Embedder)(nil)
var _ = (*milvus.Store)(nil)
