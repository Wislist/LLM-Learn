// Package store wraps DuckDB for structured sales/quote data and session logs.
//
// Schema:
//   sales       (order_date DATE, product TEXT, region TEXT, qty DOUBLE, amount DOUBLE, source TEXT)
//   chat_log    (ts TIMESTAMP, role TEXT, content TEXT, session TEXT)
//
// DuckDB can read .xlsx and .csv directly via `read_xlsx` / `read_csv_auto`,
// which we use during ingest to bulk-load sales detail without writing a Go parser.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

type Store struct {
	db *sql.DB
}

// New opens (or creates) the DuckDB file at path.
func New(path string) (*Store, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("store open duckdb: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("store ping duckdb: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close closes the underlying handle.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying *sql.DB for advanced use (e.g. Text-to-SQL execution).
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sales (
			order_date DATE,
			product     VARCHAR,
			region      VARCHAR,
			qty         DOUBLE,
			amount      DOUBLE,
			source      VARCHAR
		)`,
		`CREATE TABLE IF NOT EXISTS chat_log (
			ts      TIMESTAMP,
			role    VARCHAR,
			content VARCHAR,
			session VARCHAR
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("store migrate: %w", err)
		}
	}
	return nil
}

// LoadSalesFromExcel bulk-loads an xlsx file via DuckDB's read_xlsx.
// sheetName may be "" to read the first sheet.
// The xlsx must have columns matching: order_date, product, region, qty, amount
// (extra columns are ignored; headers are case-insensitive).
func (s *Store) LoadSalesFromExcel(ctx context.Context, path, sheetName, sourceTag string) (int64, error) {
	src := fmt.Sprintf("read_xlsx('%s', header=true)", path)
	if sheetName != "" {
		src = fmt.Sprintf("read_xlsx('%s', sheet='%s', header=true)", path, sheetName)
	}
	q := fmt.Sprintf(`
		INSERT INTO sales (order_date, product, region, qty, amount, source)
		SELECT
			CAST(order_date AS DATE),
			COALESCE(product,''),
			COALESCE(region,''),
			COALESCE(qty,0),
			COALESCE(amount,0),
			$1
		FROM %s
		WHERE order_date IS NOT NULL`, src)
	res, err := s.db.ExecContext(ctx, q, sourceTag)
	if err != nil {
		return 0, fmt.Errorf("store load sales: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// LogChat appends a chat message to chat_log.
func (s *Store) LogChat(ctx context.Context, role, content, session string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_log (ts, role, content, session) VALUES (?, ?, ?, ?)`,
		time.Now().UTC(), role, content, session)
	return err
}

// ChartPoint is a single bucket for a chart series.
type ChartPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// RunAggSQL runs a SELECT that returns label,value rows and returns them as chart points.
// Used by the text-to-sql handler.
func (s *Store) RunAggSQL(ctx context.Context, query string) ([]ChartPoint, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("store run agg: %w", err)
	}
	defer rows.Close()
	var out []ChartPoint
	for rows.Next() {
		var p ChartPoint
		if err := rows.Scan(&p.Label, &p.Value); err != nil {
			return nil, fmt.Errorf("store scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
