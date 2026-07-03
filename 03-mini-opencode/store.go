package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wislist/llmg"
	_ "modernc.org/sqlite"
)

type Session struct {
	ID        string
	Title     string
	Model     string
	CreatedAt string
	UpdatedAt string
}

type SessionStore struct {
	db *sql.DB
}

func OpenSessionStore(path string) (*SessionStore, error) {
	if dir := filepath.Dir(path); dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open session db: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &SessionStore{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SessionStore) Close() error { return s.db.Close() }

func (s *SessionStore) init() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,
  title      TEXT,
  model      TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS messages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL REFERENCES sessions(id),
  seq        INTEGER NOT NULL,
  payload    TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, seq);
`)
	return err
}

func (s *SessionStore) CreateSession(title, model string) (string, error) {
	id := newSessionID()
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, title, model, updated_at) VALUES (?, ?, ?, ?)`,
		id, title, model, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return id, nil
}

func (s *SessionStore) RenameSession(id, title string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET title=?, updated_at=? WHERE id=?`,
		title, time.Now().Format(time.RFC3339), id,
	)
	return err
}

func (s *SessionStore) AppendMessage(sessionID string, seq int, msg llmg.Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO messages (session_id, seq, payload) VALUES (?, ?, ?)`,
		sessionID, seq, string(payload),
	)
	if err != nil {
		return fmt.Errorf("append message: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE sessions SET updated_at=? WHERE id=?`,
		time.Now().Format(time.RFC3339), sessionID,
	)
	return err
}

func (s *SessionStore) LoadMessages(sessionID string) ([]llmg.Message, error) {
	rows, err := s.db.Query(
		`SELECT payload FROM messages WHERE session_id=? ORDER BY seq ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}
	defer rows.Close()

	var out []llmg.Message
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var msg llmg.Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			return nil, fmt.Errorf("unmarshal message: %w", err)
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *SessionStore) LatestSession() (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, title, model, created_at, updated_at
		 FROM sessions ORDER BY updated_at DESC LIMIT 1`,
	)
	var sess Session
	err := row.Scan(&sess.ID, &sess.Title, &sess.Model, &sess.CreatedAt, &sess.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SessionStore) ListSessions(limit int) ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, title, model, created_at, updated_at
		 FROM sessions ORDER BY updated_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.Title, &sess.Model, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *SessionStore) GetSession(id string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, title, model, created_at, updated_at FROM sessions WHERE id=?`,
		id,
	)
	var sess Session
	err := row.Scan(&sess.ID, &sess.Title, &sess.Model, &sess.CreatedAt, &sess.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SessionStore) MessageCount(sessionID string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE session_id=?`, sessionID,
	).Scan(&n)
	return n, err
}

func newSessionID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
