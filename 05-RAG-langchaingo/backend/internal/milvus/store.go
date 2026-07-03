// Package milvus wraps the Milvus Go SDK v2 to give us a small, explicit API
// for the two collections this app uses:
//   - documents: chunked text from docx/pdf/excel narrative cells
//   - quotes   : per-line historical quote records (project + item + price)
//
// Both collections share the same schema shape: a pk int64, a text field,
// metadata fields, and a 1024-dim float vector (bge-m3).
package milvus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	VectorField = "vector"
	Dim         = 1024

	PKField     = "id"
	SourceField = "source"
	TypeField   = "doc_type"
	TextField   = "content"
	// quote-specific metadata
	ProjectField = "project_name"
	ItemField    = "item_name"
	QtyField     = "qty"
	UnitField    = "unit_price"
	TotalField   = "total"
)

// Store is a thin wrapper over a Milvus client.
type Store struct {
	c        client.Client
	collDocs string
	collQ   string
}

// New connects to Milvus at addr and remembers the collection names.
func New(addr, collDocs, collQuotes string) (*Store, error) {
	c, err := client.NewClient(
		context.Background(),
		client.Config{Address: addr},
	)
	if err != nil {
		return nil, fmt.Errorf("milvus connect %s: %w", addr, err)
	}
	return &Store{c: c, collDocs: collDocs, collQ: collQuotes}, nil
}

// Close releases the underlying connection.
func (s *Store) Close() error { return s.c.Close() }

// EnsureCollections creates the two collections if they don't exist and loads
// them into memory for search.
func (s *Store) EnsureCollections(ctx context.Context) error {
	for name, schema := range map[string]*entity.Schema{
		s.collDocs: docsSchema(s.collDocs),
		s.collQ:    quotesSchema(s.collQ),
	} {
		exists, err := s.c.HasCollection(ctx, name)
		if err != nil {
			return fmt.Errorf("milvus has collection %s: %w", name, err)
		}
		if !exists {
			if err := s.c.CreateCollection(ctx, schema, 2 /* shards */); err != nil {
				return fmt.Errorf("milvus create %s: %w", name, err)
			}
			if err := s.createIndex(ctx, name); err != nil {
				return fmt.Errorf("milvus index %s: %w", name, err)
			}
		}
		if err := s.c.LoadCollection(ctx, name, false); err != nil {
			return fmt.Errorf("milvus load %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) createIndex(ctx context.Context, coll string) error {
	idx, err := entity.NewIndexIvfFlat(entity.IP, 1024)
	if err != nil {
		return fmt.Errorf("new index: %w", err)
	}
	return s.c.CreateIndex(ctx, coll, VectorField, idx, false)
}

// ---------- schemas ----------

func docsSchema(name string) *entity.Schema {
	return entity.NewSchema().
		WithName(name).
		WithAutoID(true).
		WithDescription("chunked text from uploaded documents").
		WithField(entity.NewField().WithName(PKField).WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
		WithField(entity.NewField().WithName(SourceField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(256)).
		WithField(entity.NewField().WithName(TypeField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(32)).
		WithField(entity.NewField().WithName(TextField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(8192)).
		WithField(entity.NewField().WithName(VectorField).WithDataType(entity.FieldTypeFloatVector).WithDim(Dim))
}

func quotesSchema(name string) *entity.Schema {
	return entity.NewSchema().
		WithName(name).
		WithAutoID(true).
		WithDescription("historical quote line items").
		WithField(entity.NewField().WithName(PKField).WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
		WithField(entity.NewField().WithName(ProjectField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(256)).
		WithField(entity.NewField().WithName(ItemField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(512)).
		WithField(entity.NewField().WithName(QtyField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(64)).
		WithField(entity.NewField().WithName(UnitField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(64)).
		WithField(entity.NewField().WithName(TotalField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(64)).
		WithField(entity.NewField().WithName(SourceField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(256)).
		WithField(entity.NewField().WithName(TextField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(8192)).
		WithField(entity.NewField().WithName(VectorField).WithDataType(entity.FieldTypeFloatVector).WithDim(Dim))
}

// ---------- types ----------

// DocRow is one chunk to insert into the documents collection.
type DocRow struct {
	Source  string
	Type    string
	Content string
	Vector  []float32
}

// QuoteRow is one line item to insert into the quotes collection.
type QuoteRow struct {
	Project string
	Item    string
	Qty     string
	Unit    string
	Total   string
	Source  string
	Content string // textualized row used for embedding
	Vector  []float32
}

// SearchHit is a generic search result row.
type SearchHit struct {
	IDStr   string
	Score   float32
	Content string
	Source  string
	Fields  map[string]string
}

// ---------- insert ----------

func (s *Store) InsertDocs(ctx context.Context, rows []DocRow) error {
	if len(rows) == 0 {
		return nil
	}
	src := make([]string, len(rows))
	typ := make([]string, len(rows))
	txt := make([]string, len(rows))
	vec := make([][]float32, len(rows))
	for i, r := range rows {
		src[i] = r.Source
		typ[i] = r.Type
		txt[i] = r.Content
		vec[i] = r.Vector
	}
	col, err := s.c.Insert(ctx, s.collDocs, "",
		entity.NewColumnVarChar(SourceField, src),
		entity.NewColumnVarChar(TypeField, typ),
		entity.NewColumnVarChar(TextField, txt),
		entity.NewColumnFloatVector(VectorField, Dim, vec),
	)
	if err != nil {
		return fmt.Errorf("insert docs: %w", err)
	}
	_ = col
	return nil
}

func (s *Store) InsertQuotes(ctx context.Context, rows []QuoteRow) error {
	if len(rows) == 0 {
		return nil
	}
	proj := make([]string, len(rows))
	item := make([]string, len(rows))
	qty := make([]string, len(rows))
	unit := make([]string, len(rows))
	total := make([]string, len(rows))
	src := make([]string, len(rows))
	txt := make([]string, len(rows))
	vec := make([][]float32, len(rows))
	for i, r := range rows {
		proj[i] = r.Project
		item[i] = r.Item
		qty[i] = r.Qty
		unit[i] = r.Unit
		total[i] = r.Total
		src[i] = r.Source
		txt[i] = r.Content
		vec[i] = r.Vector
	}
	_, err := s.c.Insert(ctx, s.collQ, "",
		entity.NewColumnVarChar(ProjectField, proj),
		entity.NewColumnVarChar(ItemField, item),
		entity.NewColumnVarChar(QtyField, qty),
		entity.NewColumnVarChar(UnitField, unit),
		entity.NewColumnVarChar(TotalField, total),
		entity.NewColumnVarChar(SourceField, src),
		entity.NewColumnVarChar(TextField, txt),
		entity.NewColumnFloatVector(VectorField, Dim, vec),
	)
	if err != nil {
		return fmt.Errorf("insert quotes: %w", err)
	}
	return nil
}

// ---------- search ----------

// SearchDocs searches the documents collection by vector.
func (s *Store) SearchDocs(ctx context.Context, vec []float32, topK int) ([]SearchHit, error) {
	return s.search(ctx, s.collDocs, vec, topK, []string{SourceField, TypeField, TextField})
}

// SearchQuotes searches the quotes collection by vector.
func (s *Store) SearchQuotes(ctx context.Context, vec []float32, topK int) ([]SearchHit, error) {
	return s.search(ctx, s.collQ, vec, topK, []string{ProjectField, ItemField, QtyField, UnitField, TotalField, SourceField, TextField})
}

func (s *Store) search(ctx context.Context, coll string, vec []float32, topK int, outFields []string) ([]SearchHit, error) {
	sp, _ := entity.NewIndexIvfFlatSearchParam(16)
	res, err := s.c.Search(ctx, coll, nil, "", outFields,
		[]entity.Vector{entity.FloatVector(vec)},
		VectorField,
		entity.IP,
		topK,
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("milvus search: %w", err)
	}
	if len(res) == 0 || res[0].ResultCount == 0 {
		return nil, nil
	}
	sr := res[0]
	hits := make([]SearchHit, 0, sr.ResultCount)
	for i := 0; i < sr.ResultCount; i++ {
		h := SearchHit{}
		if i < len(sr.Scores) {
			h.Score = sr.Scores[i]
		}
		if sr.IDs != nil {
			if v, err := sr.IDs.GetAsString(i); err == nil {
				h.IDStr = v
			}
		}
		for _, f := range sr.Fields {
			name := f.Name()
			val, err := f.GetAsString(i)
			if err != nil {
				continue
			}
			switch name {
			case TextField:
				h.Content = val
			case SourceField:
				h.Source = val
			}
			if h.Fields == nil {
				h.Fields = map[string]string{}
			}
			h.Fields[name] = val
		}
		hits = append(hits, h)
	}
	return hits, nil
}

// Ping does a trivial HasCollection to verify connectivity.
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.c.HasCollection(ctx, s.collDocs)
	return err
}

// guard against accidental misuse when caller expects at least one row.
var ErrEmpty = errors.New("milvus: empty result")
